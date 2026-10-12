module example.com/app

go 1.27

require github.com/spf13/cobra v1.10.2

require (
	github.com/danielgtaylor/huma/v2 v2.39.1
	// a comment on its own line
	github.com/jackc/pgx/v5 v5.11.0 // indirect
)

replace github.com/spf13/cobra => ../cobra

tool golang.org/x/tools/cmd/stringer
