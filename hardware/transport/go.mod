module github.com/meshcore-go/meshcore-go/hardware/transport

go 1.26.1

require (
	github.com/meshcore-go/meshcore-go v1.0.8
	go.bug.st/serial v1.6.4
)

require (
	github.com/creack/goselect v0.1.2 // indirect
	golang.org/x/sys v0.42.0 // indirect
)

replace github.com/meshcore-go/meshcore-go => ../../

retract v1.0.7 // tagged from the wrong commit, then re-tagged; use v1.0.8 instead
