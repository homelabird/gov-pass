module fk-gov

go 1.21

require (
	github.com/florianl/go-nfqueue v1.3.2
	github.com/mdlayher/netlink v1.6.0
	golang.org/x/sys v0.15.0
)

require (
	github.com/google/go-cmp v0.5.7 // indirect
	github.com/josharian/native v1.0.0 // indirect
	github.com/mdlayher/socket v0.1.1 // indirect
	golang.org/x/net v0.0.0-20220127200216-cd36cc0744dd // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
)

replace github.com/florianl/go-nfqueue => ./third_party/go-nfqueue

replace github.com/mdlayher/netlink => ./third_party/netlink
