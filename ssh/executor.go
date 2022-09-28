package ssh

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os/user"
	"strconv"
	"time"

	"github.com/seveas/herd"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

type Executor struct {
	agent          *agent
	config         *config
	connectTimeout time.Duration
}

func NewExecutor(agentTimeout time.Duration, user user.User) (herd.Executor, error) {
	agent, err := newAgent(agentTimeout)
	if err != nil {
		return nil, err
	}
	config := newConfig(user)
	if err := config.readOpenSSHConfig(); err != nil {
		return nil, err
	}

	return &Executor{
		agent:  agent,
		config: config,
	}, nil
}

func (e *Executor) SetConnectTimeout(t time.Duration) {
	e.connectTimeout = t
}

func (e *Executor) Run(ctx context.Context, host *herd.Host, command string, oc chan herd.OutputLine) *herd.Result {
	now := time.Now()
	r := &herd.Result{Host: host, StartTime: now, EndTime: now, ElapsedTime: 0, ExitStatus: -1}
	defer func() {
		r.EndTime = time.Now()
		r.ElapsedTime = r.EndTime.Sub(r.StartTime).Seconds()
	}()

	if err := ctx.Err(); err != nil {
		r.Err = err
		return r
	}
	connection, err := e.connect(ctx, host)
	if err != nil {
		r.Err = err
		return r
	}
	sess, err := connection.NewSession()
	if err != nil {
		r.Err = err
		return r
	}
	defer sess.Close()

	var stdout, stderr herd.ByteWriter
	if oc != nil {
		stdout = herd.NewLineWriterBuffer(host, false, oc)
		stderr = herd.NewLineWriterBuffer(host, true, oc)
	} else {
		stdout = bytes.NewBuffer([]byte{})
		stderr = bytes.NewBuffer([]byte{})
	}

	sess.Stdout = stdout
	sess.Stderr = stderr
	ec := make(chan error)

	go func() {
		ec <- sess.Run(command)
	}()

	select {
	case <-ctx.Done():
		// FIXME: no error is ever returned, but the signal is not sent to the process either.
		// https://github.com/openssh/openssh-portable/commit/cd98925c6405e972dc9f211afc7e75e838abe81c
		// - OpenSSH 7.9 or newer required
		sess.Signal(ssh.SIGKILL)
		r.Err = herd.TimeoutError{Message: "Timed out while executing command"}
	case err := <-ec:
		r.Err = err
	}
	if r.Err != nil {
		if err, ok := r.Err.(*ssh.ExitError); ok {
			r.ExitStatus = err.ExitStatus()
		}
	} else {
		r.ExitStatus = 0
	}
	r.Stdout = stdout.Bytes()
	r.Stderr = stderr.Bytes()
	return r
}

func (e *Executor) connect(ctx context.Context, host *herd.Host) (*ssh.Client, error) {
	if host.Connection != nil {
		return host.Connection.(*ssh.Client), nil
	}

	config := e.config.forHost(host)
	cc := config.clientConfig
	cc.HostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return e.hostKeyCallback(host, key, config)
	}
	cc.Auth = []ssh.AuthMethod{ssh.PublicKeysCallback(e.agent.SignersForPathCallback(config.identityFile))}

	address := host.Address
	if address == "" {
		address = host.Name
	}
	address = net.JoinHostPort(address, strconv.Itoa(config.port))
	logrus.Debugf("Connecting to %s (%s) as %s with key %s", host.Name, address, cc.User, config.identityFile)

	ctx, cancel := context.WithTimeout(ctx, e.connectTimeout+time.Second/2)
	defer cancel()
	var client *ssh.Client
	ec := make(chan error)
	go func() {
		var err error
		client, err = ssh.Dial("tcp", address, cc)
		ec <- err
	}()
	select {
	case <-ctx.Done():
		return nil, herd.TimeoutError{Message: "Timed out while connecting to server"}
	case err := <-ec:
		if err == nil {
			host.Connection = client
		}
		return client, err
	}
}

func (e *Executor) hostKeyCallback(host *herd.Host, key ssh.PublicKey, c *configBlock) error {
	// Do we have the key?
	bkey := key.Marshal()
	for _, pkey := range host.PublicKeys() {
		if bytes.Equal(bkey, pkey.Marshal()) {
			return nil
		}
	}

	// We don't have the key, but is it in DNS?
	if c.verifyHostKeyDns && verifyHostKeyDns(host.Name, key) {
		host.AddPublicKey(key)
		return nil
	}

	switch c.strictHostKeyChecking {
	case acceptNew:
		logrus.Warnf("ssh: no known host key for %s, accepting new key", host.Name)
		fallthrough
	case no:
		host.AddPublicKey(key)
		return nil
	default:
		return fmt.Errorf("ssh: no host key found for %s", host.Name)
	}
}

var _ herd.Executor = &Executor{}
