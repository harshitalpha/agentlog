#!/bin/sh
# Builds a realistic demo repo for the UI, the e2e tests and manual checks.
#
#   scripts/make-demo-repo.sh <dir>        (build the binary first: make build)
#
# Resulting history, oldest first:
#   1. Baseline snapshot before agent work          (agentlog init)
#   2. Add agentlog commit rules for AI agents      (agentlog init)
#   3. Add controller layer for the orders API      Agent: Claude
#   4. Add ProductRepository interface and Postgres impl   Agent: Claude
#   5. Add GetProductsByDate query to product DB    Agent: Claude
#   6. Fix empty-id handling in ProductRepository.Get      Agent: Claude (edits the file from 4)
#   7. Update README wording                        human, author "Claude Dev", no trailer
#   8. [auto] uncommitted changes: x.go             produced by the Stop hook's second attempt
# plus 2 uncommitted changes: one modified file and one untracked file.
set -eu
dir="${1:?usage: make-demo-repo.sh <dir>}"
here="$(cd "$(dirname "$0")" && pwd)"
bin="${AGENTLOG_BIN:-$here/../bin/agentlog}"
[ -x "$bin" ] || { echo "make-demo-repo: $bin not found. Run 'make build' first." >&2; exit 1; }

rm -rf "$dir"
mkdir -p "$dir"
cd "$dir"

# A small Go project that exists before any agent work.
cat > go.mod <<'EOT'
module demo

go 1.22
EOT
cat > main.go <<'EOT'
package main

func main() {}
EOT
cat > README.md <<'EOT'
# demo

A tiny service used to try out agentlog.
EOT

"$bin" init "$dir" >/dev/null
# Never sign or prompt in a fixture, whatever the user's global config says.
git config --local commit.gpgsign false

agent_commit() { # subject, body
  git add -A
  git commit -q -m "$1" -m "$2

Agent: Claude"
}

mkdir -p internal/api internal/repo

cat > internal/api/orders.go <<'EOT'
package api

import "net/http"

// OrdersHandler serves the orders API.
func OrdersHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
EOT
agent_commit "Add controller layer for the orders API" "Routes order requests to a single handler so the service layer can grow behind it."

cat > internal/repo/product.go <<'EOT'
package repo

// Product is a catalogue entry.
type Product struct {
	ID   string
	Name string
}

// ProductRepository stores and loads products.
type ProductRepository interface {
	Get(id string) (*Product, error)
}
EOT
cat > internal/repo/product_pg.go <<'EOT'
package repo

type pgProductRepo struct{}

func (r *pgProductRepo) Get(id string) (*Product, error) { return nil, nil }
EOT
agent_commit "Add ProductRepository interface and Postgres impl" "The interface keeps handlers independent of the database."

cat >> internal/repo/product_pg.go <<'EOT'

func (r *pgProductRepo) GetByDate(date string) ([]Product, error) {
	return nil, nil
}
EOT
agent_commit "Add GetProductsByDate query to product DB" "Needed for the daily catalogue export."

cat > internal/repo/product_pg.go <<'EOT'
package repo

import "errors"

type pgProductRepo struct{}

func (r *pgProductRepo) Get(id string) (*Product, error) {
	if id == "" {
		return nil, errors.New("empty product id")
	}
	return nil, nil
}

func (r *pgProductRepo) GetByDate(date string) ([]Product, error) {
	return nil, nil
}
EOT
agent_commit "Fix empty-id handling in ProductRepository.Get" "Get used to hit the database with an empty key. Follow-up: add a test."

# A human commit by someone whose name happens to contain "Claude": it must
# NOT be badged as an agent commit (there is no trailer).
printf '\nRun with: go run .\n' >> README.md
git add -A
git -c user.name="Claude Dev" -c user.email="dev@example.com" commit -q -m "Update README wording"

# The Stop hook: first attempt blocks, second attempt auto-commits.
echo 'package main' > x.go
first="$("$here/fake-claude-stop.sh" "$dir" false demo-session)"
case "$first" in *'"decision":"block"'*) ;; *) echo "make-demo-repo: expected the Stop hook to block, got: $first" >&2; exit 1 ;; esac
"$here/fake-claude-stop.sh" "$dir" true demo-session >/dev/null

# Leave live uncommitted changes: one modified, one untracked.
printf '\n// TODO: validate order payloads\n' >> internal/api/orders.go
printf 'scratch notes\n' > notes.txt

echo "demo repo ready: $dir"
