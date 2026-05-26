module spore-shell

go 1.25.0

require (
	github.com/sporeos-dev/spore-client-libs/go v0.0.0
	golang.org/x/term v0.41.0
)

require golang.org/x/sys v0.42.0 // indirect

replace github.com/sporeos-dev/spore-client-libs/go => ../../spore-client-libs/go
