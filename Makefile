# Let's not rebuild the parser if we don't have antlr available
ifeq ("", "$(strip $(shell which antlr))")
	antlr_sources :=
else
	antlr_sources := scripting/parser/herd_base_listener.go scripting/parser/herd_lexer.go scripting/parser/herd_listener.go scripting/parser/herd_parser.go
endif

# Let's not rebuild the protobuf code if we don't have protobuf available
ifeq ("", "$(strip $(shell which protoc))")
	protobuf_sources :=
else ifeq ("", "$(strip $(shell which protoc-gen-go))")
	protobuf_sources :=
else ifeq ("", "$(strip $(shell which protoc-gen-go-crpc))")
	protobuf_sources :=
else
	protobuf_sources = provider/plugin/common/plugin.pb.go provider/plugin/common/plugin_grpc.pb.go
endif

herd: go.mod *.go cmd/herd/*.go scripting/*.go provider/*/*.go provider/plugin/common/*.go $(protobuf_sources) $(antlr_sources)
	go build -o "$@" github.com/seveas/herd/cmd/herd

%_grpc.pb.go: %.proto
	protoc --go-grpc_out=. $^

%.pb.go: %.proto
	protoc --go_out=. $^

herd-provider-%: cmd/herd-provider-%/*.go provider/%/*.go provider/plugin/common/* provider/plugin/server/* $(protobuf_sources)
	go build -o "$@" github.com/seveas/herd/cmd/$@

ssh-agent-proxy: go.mod cmd/ssh-agent-proxy/*.go
	go build -o "$@" github.com/seveas/herd/cmd/ssh-agent-proxy

$(antlr_sources): scripting/Herd.g4
	(cd scripting; antlr -Dlanguage=Go -o parser Herd.g4)

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

provider/plugin/testdata/bin/herd-provider-ci: provider/plugin/testdata/provider/ci/*.go provider/plugin/testdata/cmd/herd-provider-ci/*.go provider/plugin/common/* provider/plugin/server/* $(protobuf_sources)
	go build -o "$@" github.com/seveas/herd/provider/plugin/testdata/cmd/herd-provider-ci

test: fmt vet tidy provider/plugin/testdata/bin/herd-provider-ci
	go test ./...

test-integration:
	go mod vendor
	make -C integration/pki
	test -e integration/openssh/user.key || ssh-keygen -t ecdsa -f integration/openssh/user.key -N ""
	docker-compose down || true
	docker-compose build
	docker-compose up --exit-code-from herd --abort-on-container-exit
	docker-compose down

dist_oses := darwin dragonfly freebsd linux netbsd openbsd windows
ssh_agent_oses := darwin dragonfly freebsd linux netbsd openbsd
build_all:
	@echo Building herd
	@$(foreach os,$(dist_oses),echo " - for $(os)" && mkdir -p dist/$(os)-amd64 && GOOS=$(os) GOARCH=amd64 go build -tags no_extra -ldflags '-s -w' -o dist/$(os)-amd64/ github.com/seveas/herd/cmd/herd;)
	@echo Building ssh-agent-proxy
	@$(foreach os,$(ssh_agent_oses),echo " - for $(os)" && GOOS=$(os) GOARCH=amd64 go build -ldflags '-s -w' -o dist/$(os)-amd64/ github.com/seveas/herd/cmd/ssh-agent-proxy;)

clean:
	rm -f herd
	rm -f herd-provider-example
	rm -f ssh-agent-proxy
	go mod tidy

fullclean: clean
	rm -rf dist/
	rm -f $(antlr_sources)
