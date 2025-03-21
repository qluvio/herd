package plugin

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/seveas/herd"
	"github.com/seveas/herd/provider/plugin/common"

	"github.com/hashicorp/go-plugin"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

func init() {
	herd.RegisterProvider("plugin", newPlugin, nil)
}

type pluginProvider struct {
	name     string
	settings map[string]interface{}
	config   struct {
		Command string
		Prefix  string
	}
}

func newPlugin(name string) herd.HostProvider {
	p := &pluginProvider{name: name}
	if path, err := exec.LookPath(fmt.Sprintf("herd-provider-%s", name)); err == nil {
		p.config.Command = path
	}
	if path, err := exec.LookPath(fmt.Sprintf("herd-provider-%s.exe", name)); err == nil {
		p.config.Command = path
	}
	p.settings = map[string]interface{}{"name": name}
	return p
}

func (p *pluginProvider) Name() string {
	return p.name
}

func (p *pluginProvider) Prefix() string {
	return p.config.Prefix
}

func (p *pluginProvider) ParseViper(v *viper.Viper) error {
	p.settings = v.AllSettings()
	p.settings["herd_provider_name"] = p.name
	return v.Unmarshal(&p.config)
}

func (p *pluginProvider) Equivalent(o herd.HostProvider) bool {
	return p.config.Command == o.(*pluginProvider).config.Command
}

func (p *pluginProvider) Load(ctx context.Context, lm herd.LoadingMessage) (herd.Hosts, error) {
	pluginMap := map[string]plugin.Plugin{
		"provider": &common.ProviderPlugin{},
	}
	client := plugin.NewClient(&plugin.ClientConfig{
		Managed:          true,
		HandshakeConfig:  common.Handshake,
		Plugins:          pluginMap,
		Cmd:              exec.Command(p.config.Command),
		Logger:           common.NewLogrusLogger(logrus.StandardLogger(), fmt.Sprintf("plugin-%s", p.name)),
		SyncStdout:       os.Stdout,
		SyncStderr:       os.Stderr,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})

	rpcClient, err := client.Client()
	if err != nil {
		return nil, err
	}
	raw, err := rpcClient.Dispense("provider")
	if err != nil {
		return nil, err
	}
	pp := raw.(common.Provider)
	if err := pp.Configure(p.settings); err != nil {
		return nil, err
	}
	return pp.Load(ctx, &logForwarder{provider: p, lm: lm})
}

type logForwarder struct {
	provider *pluginProvider
	lm       herd.LoadingMessage
}

func (l *logForwarder) LoadingMessage(name string, done bool, err error) {
	l.lm(l.provider.name, done, err)
}

func (l *logForwarder) EmitLogMessage(level logrus.Level, message string) {
	logrus.StandardLogger().Log(level, message)
}

var _ common.Logger = &logForwarder{}
