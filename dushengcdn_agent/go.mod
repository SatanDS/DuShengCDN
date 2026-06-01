module dushengcdn-agent

go 1.25.0

require (
	dushengcdn v0.0.0
	github.com/miekg/dns v1.1.72
	golang.org/x/net v0.53.0
)

require (
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/tools v0.44.0 // indirect
)

replace dushengcdn => ../dushengcdn_server
