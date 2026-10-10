package taskmgr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/storecatalog"
)

// isolateCentralHome points the SDK at a registry of this test's own, so the
// listing does not depend on what the developer running the suite has
// registered on their machine.
func isolateCentralHome(t *testing.T) {
	t.Helper()
	t.Setenv("TASKMGR_HOME", filepath.Join(t.TempDir(), "taskmgr-home"))
	t.Setenv("TASKMGR_DIR", "")
}

func TestStoresListsRegisteredCentralStores(t *testing.T) {
	isolateCentralHome(t)

	project := t.TempDir()
	if _, err := tasks.InitCentral(project, "listed-fixture", "lst"); err != nil {
		t.Fatalf("tasks.InitCentral: %v", err)
	}

	entries, err := New("tester").Stores(context.Background())
	if err != nil {
		t.Fatalf("Stores: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("listed %d entries, want 1: %+v", len(entries), entries)
	}

	entry := entries[0]
	if entry.Name != "listed-fixture" {
		t.Errorf("Name: got %q, want %q", entry.Name, "listed-fixture")
	}
	if entry.StorePath == "" {
		t.Error("StorePath is empty; the picker needs it to mark the active store")
	}
	if entry.ProjectPath == "" {
		t.Error("ProjectPath is empty; the picker shows it as the row's identity")
	}
	if entry.Health != storecatalog.HealthOK {
		t.Errorf("Health: got %q, want %q", entry.Health, storecatalog.HealthOK)
	}
}

// A registry entry whose store directory has been removed is reported as
// dangling rather than dropped: the picker names it so the operator can see
// why it never opens.
func TestStoresReportsADanglingEntry(t *testing.T) {
	isolateCentralHome(t)

	project := t.TempDir()
	if _, err := tasks.InitCentral(project, "dangling-fixture", "dng"); err != nil {
		t.Fatalf("tasks.InitCentral: %v", err)
	}

	before, err := New("tester").Stores(context.Background())
	if err != nil {
		t.Fatalf("Stores: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("listed %d entries, want 1", len(before))
	}
	if err := os.RemoveAll(before[0].StorePath); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	after, err := New("tester").Stores(context.Background())
	if err != nil {
		t.Fatalf("Stores: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("listed %d entries after removing the store directory, want the entry to remain", len(after))
	}
	if after[0].Health.Usable() {
		t.Errorf("Health: got %q, want an unusable health", after[0].Health)
	}
	if after[0].Detail != "" {
		t.Errorf("Detail: got %q, want none for a directory that is absent", after[0].Detail)
	}
}

// A store directory the process may not read is reported as broken, with the
// reason, and costs no other store its place in the listing.
func TestStoresReportsAnUnreadableStoreAsBrokenAndKeepsTheRest(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory mode")
	}
	isolateCentralHome(t)

	for _, name := range []string{"readable-fixture", "refused-fixture"} {
		if _, err := tasks.InitCentral(t.TempDir(), name, "fix"); err != nil {
			t.Fatalf("tasks.InitCentral(%s): %v", name, err)
		}
	}

	before, err := New("tester").Stores(context.Background())
	if err != nil {
		t.Fatalf("Stores: %v", err)
	}
	for _, entry := range before {
		if entry.Name != "refused-fixture" {
			continue
		}
		storePath := entry.StorePath
		if err := os.Chmod(storePath, 0); err != nil {
			t.Fatalf("Chmod: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(storePath, 0o755) })
	}

	after, err := New("tester").Stores(context.Background())
	if err != nil {
		t.Fatalf("Stores with one unreadable store directory: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("listed %d entries, want both: %+v", len(after), after)
	}
	for _, entry := range after {
		switch entry.Name {
		case "readable-fixture":
			if entry.Health != storecatalog.HealthOK || entry.Detail != "" {
				t.Errorf("readable entry: got health %q and detail %q, want %q and no detail", entry.Health, entry.Detail, storecatalog.HealthOK)
			}
		case "refused-fixture":
			if entry.Health != storecatalog.HealthBroken {
				t.Errorf("unreadable entry: got health %q, want %q", entry.Health, storecatalog.HealthBroken)
			}
			if !strings.Contains(entry.Detail, "permission denied") {
				t.Errorf("unreadable entry: got detail %q, want the cause", entry.Detail)
			}
		default:
			t.Errorf("unexpected entry %q", entry.Name)
		}
	}
}

func TestStoresOnAnEmptyRegistryIsEmptyNotAnError(t *testing.T) {
	isolateCentralHome(t)

	entries, err := New("tester").Stores(context.Background())
	if err != nil {
		t.Fatalf("Stores: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("listed %d entries on an empty registry, want none", len(entries))
	}
}

func TestStoresHonoursACancelledContext(t *testing.T) {
	isolateCentralHome(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := New("tester").Stores(ctx); err == nil {
		t.Error("expected a cancelled context to be reported")
	}
}

func TestConvertHealthCoversEverySDKCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   tasks.StoreHealth
		want storecatalog.Health
	}{
		{tasks.StoreOK, storecatalog.HealthOK},
		{tasks.StoreDangling, storecatalog.HealthDangling},
		{tasks.StoreBroken, storecatalog.HealthBroken},
		// A case this build does not know about, which is what a newer SDK
		// adding a fourth one looks like from here.
		{tasks.StoreHealth(99), storecatalog.HealthUnknown},
	}
	for _, tc := range cases {
		if got := convertHealth(tc.in); got != tc.want {
			t.Errorf("convertHealth(%v): got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOpenResolvesARegisteredStoreByName(t *testing.T) {
	isolateCentralHome(t)

	project := t.TempDir()
	if _, err := tasks.InitCentral(project, "opened-fixture", "opn"); err != nil {
		t.Fatalf("tasks.InitCentral: %v", err)
	}

	opened, err := New("tester").Open(context.Background(), "opened-fixture")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if opened.Repo == nil {
		t.Fatal("Open returned no repository")
	}
	if opened.Name != "opened-fixture" {
		t.Errorf("Name: got %q, want the registry name", opened.Name)
	}
	if opened.StorePath == "" || opened.ProjectPath == "" {
		t.Errorf("paths: got store %q, project %q; both are needed once the store is active", opened.StorePath, opened.ProjectPath)
	}

	// The repository reads the store that was opened, not some other one.
	if err := opened.Repo.HealthCheck(context.Background()); err != nil {
		t.Errorf("HealthCheck on the opened store: %v", err)
	}
}

func TestOpenAnUnregisteredNameFails(t *testing.T) {
	isolateCentralHome(t)

	_, err := New("tester").Open(context.Background(), "not-registered")
	if err == nil {
		t.Fatal("expected an error opening an unregistered store")
	}
	if !strings.Contains(err.Error(), "not-registered") {
		t.Errorf("error should name the store, got: %v", err)
	}
}

// A local store's directory is always .tasks, so the header would call every
// local store ".tasks". It is named after its project instead; a central store
// keeps its registry name.
func TestStoreNameForLocalAndCentralStores(t *testing.T) {
	t.Parallel()

	local := tasks.ResolveInfo{Kind: tasks.ResolvedLocal, ProjectPath: "/home/hans/dev/widget", StorePath: "/home/hans/dev/widget/.tasks"}
	if got := StoreName(local); got != "widget" {
		t.Errorf("local store name: got %q, want the project directory's name", got)
	}

	for _, kind := range []tasks.ResolveKind{tasks.ResolvedCentral, tasks.ResolvedOverrideName} {
		central := tasks.ResolveInfo{Kind: kind, ProjectPath: "/home/hans/dev/widget", StorePath: "/home/hans/.taskmgr/stores/widget-tracker"}
		if got := StoreName(central); got != "widget-tracker" {
			t.Errorf("central store name (%v): got %q, want the registry name", kind, got)
		}
	}
}

// A directory name may hold a newline or a tab. The name is drawn on one line,
// so each run of control characters is one space.
func TestStoreNameReplacesControlCharacters(t *testing.T) {
	t.Parallel()

	info := tasks.ResolveInfo{Kind: tasks.ResolvedLocal, ProjectPath: "/home/hans/dev/\ta\nb\t\x1bc\n"}
	if got := StoreName(info); got != "a b c" {
		t.Errorf("store name: got %q, want %q", got, "a b c")
	}
}

// A bidirectional override in a directory name would reorder the menu bar
// beside it, and a zero-width space would draw a name nobody can see. The
// joiner of an emoji sequence stays.
func TestStoreNameDropsFormatCharacters(t *testing.T) {
	t.Parallel()

	for path, want := range map[string]string{
		"/dev/a\u202eb":                   "ab",
		"/dev/\u200b\ufeff":               "",
		"/dev/\U0001F468\u200d\U0001F4BB": "\U0001F468\u200d\U0001F4BB",
		// A name of joiners alone draws no cell, so it is no name.
		"/dev/" + string(rune(0x200d)): "",
	} {
		info := tasks.ResolveInfo{Kind: tasks.ResolvedLocal, ProjectPath: path}
		if got := StoreName(info); got != want {
			t.Errorf("store name of %q: got %q, want %q", path, got, want)
		}
	}
}

// A local store created from the picker is the store a later start in that
// directory finds — what `taskmgr where` would report there.
func TestCreateLocalMakesTheStoreADirectoryResolvesTo(t *testing.T) {
	isolateCentralHome(t)

	dir := t.TempDir()
	opened, err := New("tester").CreateLocal(context.Background(), dir, "loc")
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}

	_, info, err := tasks.Resolve(tasks.ResolveOptions{WorkDir: dir})
	if err != nil {
		t.Fatalf("the created store does not resolve from its directory: %v", err)
	}
	if info.Kind != tasks.ResolvedLocal || info.StorePath != opened.StorePath {
		t.Errorf("resolved %v store at %q, want the local store created at %q", info.Kind, info.StorePath, opened.StorePath)
	}
	if opened.Name != filepath.Base(dir) {
		t.Errorf("Name: got %q, want the directory's name", opened.Name)
	}
}

func TestCreateCentralRegistersTheStore(t *testing.T) {
	isolateCentralHome(t)

	dir := t.TempDir()
	opened, err := New("tester").CreateCentral(context.Background(), dir, "created-central", "cen")
	if err != nil {
		t.Fatalf("CreateCentral: %v", err)
	}
	if opened.Name != "created-central" {
		t.Errorf("Name: got %q, want the registry name", opened.Name)
	}

	entries, err := New("tester").Stores(context.Background())
	if err != nil {
		t.Fatalf("Stores: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "created-central" {
		t.Errorf("registry after the create: got %+v, want the one store just created", entries)
	}
}

// A second central store under a name already taken fails, and the error says
// which name — it is what the operator has to change in the form.
func TestCreateCentralWithATakenNameFails(t *testing.T) {
	isolateCentralHome(t)

	catalog := New("tester")
	if _, err := catalog.CreateCentral(context.Background(), t.TempDir(), "taken", "tkn"); err != nil {
		t.Fatalf("first CreateCentral: %v", err)
	}

	_, err := catalog.CreateCentral(context.Background(), t.TempDir(), "taken", "tkn")
	if err == nil {
		t.Fatal("expected a second store under the same name to fail")
	}
	if !strings.Contains(err.Error(), "taken") {
		t.Errorf("error should name the store, got: %v", err)
	}
}

func TestDerivePrefixIsTheSDKDefault(t *testing.T) {
	t.Parallel()

	if got, want := New("tester").DerivePrefix("/home/hans/dev/widget"), tasks.DerivePrefix("/home/hans/dev/widget"); got != want {
		t.Errorf("DerivePrefix: got %q, want the SDK's %q", got, want)
	}
}
