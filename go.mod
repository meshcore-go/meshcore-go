module github.com/meshcore-go/meshcore-go

go 1.26.1

require (
	filippo.io/edwards25519 v1.2.0
	golang.org/x/crypto v0.49.0
)

require (
	github.com/meshcore-go/meshcore-go/companion/transport v0.0.0
	github.com/meshcore-go/meshcore-go/hardware/transport v0.0.0
)

require (
	github.com/creack/goselect v0.1.2 // indirect
	go.bug.st/serial v1.6.4 // indirect
	golang.org/x/sys v0.42.0 // indirect
)

replace (
	github.com/meshcore-go/meshcore-go/companion/transport => ./companion/transport
	github.com/meshcore-go/meshcore-go/hardware/transport => ./hardware/transport
)
