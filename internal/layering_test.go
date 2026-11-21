package internal_test

import (
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
)

const module = "github.com/aantich/lastapp/"

// layers is the documented dependency direction (DEVELOPMENT.md, package
// layout): an internal package may import only internal packages of a strictly
// lower layer. control → worker → agent → session → dataapi → delivery → publisher → entities → inventory → membership → roles → databases
// → op → access → registry, with config, sealer and the entity/data/authz/
// artifact leaves below them. A new internal package must be placed here.
var layers = map[string]int{
	"control":     110,
	"worker":      100,
	"agent":       99,
	"session":     98,
	"dataapi":     97,
	"apppackages": 97,
	"delivery":    96,
	"publisher":   95,
	"entities":    90,
	"inventory":   80,
	"membership":  70,
	"roles":       60,
	"databases":   50,
	"httpapi":     45,
	"op":          40,
	"access":      30,
	"registry":    20,
	"sealer":      15,
	"testdb":      11,
	"config":      10,
	"data":        5,
	"authz":       4,
	"artifact":    4,
	"entity":      4,
}

// transportOnly packages are HTTP/wiring plumbing: below control, but no
// domain package may depend on them. dataapi is the HTTP-facing App data API
// (/data/v1, App launch/sessions): it serves requests itself, so it may use the
// shared httpapi plumbing (decode/send, error writer, limiter, proxies) instead
// of duplicating it, and only control (which mounts it) may import it.
// session (console login, sessions, CSRF/origin checks, password login and
// change) is HTTP-facing for the same reason: its handlers set cookies,
// redirect through OIDC and write errors with httpapi. agent (the OAuth
// authorization server for MCP clients, the console agent-access endpoints and
// the streamable-HTTP MCP server) is HTTP-facing too: it serves token,
// registration and MCP requests itself and reuses httpapi for decode/send,
// errors, admission limits, origins and client IPs. Agent OAuth consent also
// authenticates console users, but it receives the console session wrapper
// (httpapi.Session over session.Manager.Principal) from control rather than
// importing session.
var transportOnly = map[string]bool{"control": true, "agent": true, "session": true, "dataapi": true, "httpapi": true}

// mountedByControl are the HTTP-facing packages control wires and mounts. They
// are siblings: none imports another, and only control (or cmd/) imports them.
var mountedByControl = map[string]bool{"agent": true, "session": true, "dataapi": true}

type listedPackage struct {
	ImportPath   string
	Imports      []string
	TestImports  []string
	XTestImports []string
}

func TestPackageLayering(t *testing.T) {
	goBin, e := exec.LookPath("go")
	if e != nil {
		t.Fatal("go toolchain not found:", e)
	}
	cmd := exec.Command(goBin, "list", "-e", "-json=ImportPath,Imports,TestImports,XTestImports", "./...")
	cmd.Dir = ".."
	out, e := cmd.Output()
	if e != nil {
		t.Fatal("go list:", e)
	}
	d := json.NewDecoder(strings.NewReader(string(out)))
	checked := 0
	for {
		var p listedPackage
		if e := d.Decode(&p); errors.Is(e, io.EOF) {
			break
		} else if e != nil {
			t.Fatal(e)
		}
		checked++
		from := strings.TrimPrefix(p.ImportPath, module)
		fromLayer, fromInternal := internalLayer(t, from)
		seen := map[string]bool{}
		for _, imp := range append(append(append([]string{}, p.Imports...), p.TestImports...), p.XTestImports...) {
			if seen[imp] || imp == p.ImportPath || !strings.HasPrefix(imp, module+"internal/") {
				continue
			}
			seen[imp] = true
			to := strings.TrimPrefix(imp, module)
			toLayer, _ := internalLayer(t, to)
			name := strings.TrimPrefix(to, "internal/")
			switch {
			case strings.HasPrefix(from, "cmd/"):
				// Binaries wire the service together.
			case mountedByControl[name] && from != "internal/control":
				t.Errorf("%s imports %s: only control may import the packages it mounts (agent, session, dataapi)", from, to)
			case transportOnly[name] && (!fromInternal || !transportOnly[strings.TrimPrefix(from, "internal/")]):
				t.Errorf("%s imports %s: only cmd/ and transport packages (control, agent, session, dataapi) may import transport packages", from, to)
			case !fromInternal:
				// Non-internal module packages (packages/, web/) may use leaves only.
				if toLayer > layers["config"] {
					t.Errorf("%s imports %s: only cmd/ may import service layers", from, to)
				}
			case toLayer >= fromLayer:
				t.Errorf("%s imports %s: %s (layer %d) must not import an equal or higher layer (%d)", from, to, from, fromLayer, toLayer)
			}
		}
	}
	if checked == 0 {
		t.Fatal("go list returned no packages")
	}
}

// internalLayer returns the layer of an internal/<name> package; any other
// module package is reported as not internal.
func internalLayer(t *testing.T, path string) (int, bool) {
	t.Helper()
	name, ok := strings.CutPrefix(path, "internal/")
	if !ok {
		return 0, false
	}
	if i := strings.IndexByte(name, '/'); i >= 0 {
		name = name[:i]
	}
	layer, known := layers[name]
	if !known {
		t.Errorf("internal/%s is not in the layering table (internal/layering_test.go); place it in the documented dependency direction", name)
	}
	return layer, true
}
