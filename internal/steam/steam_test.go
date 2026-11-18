package steam

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledAndMods(t *testing.T) {
	d := t.TempDir()
	os.MkdirAll(filepath.Join(d, "steamapps", "workshop"), 0o755)
	os.WriteFile(filepath.Join(d, "steamapps", "appmanifest_380870.acf"), []byte("\"AppState\"\n{\n\"buildid\" \"42\"\n\"UserConfig\" { \"BetaKey\" \"unstable\" }\n}"), 0o644)
	os.WriteFile(filepath.Join(d, "steamapps", "workshop", "appworkshop_108600.acf"), []byte("\"AppWorkshop\"\n{\n\"WorkshopItemsInstalled\"\n{\n\"111\" { \"timeupdated\" \"100\" }\n\"222\" { \"timeupdated\" \"200\" }\n}\n}"), 0o644)
	b, br, err := Installed(d)
	if err != nil || b != "42" || br != "unstable" {
		t.Fatalf("%v %s %s", err, b, br)
	}
	m, err := LocalMods(d)
	if err != nil || m["111"] != 100 || m["222"] != 200 {
		t.Fatalf("%v %v", err, m)
	}
}

func TestLatest(t *testing.T) {
	run := func(...string) (string, error) {
		return "Redirecting stderr\nConnecting anonymously...\n\"380870\"\n{\n\"depots\"\n{\n\"branches\"\n{\n\"public\" { \"buildid\" \"100\" }\n\"unstable\" { \"buildid\" \"101\" }\n}\n}\n}\nsteam>", nil
	}
	if id, err := Latest(run, "unstable"); err != nil || id != "101" {
		t.Fatalf("%v %s", err, id)
	}
	if _, err := Latest(run, "nope"); err == nil {
		t.Fatal("unknown branch must error")
	}
}

func TestInstallArgs(t *testing.T) {
	if got := InstallArgs("/d", "public", false); len(got) != 7 {
		t.Fatalf("%v", got)
	}
	got := InstallArgs("/d", "unstable", true)
	if got[len(got)-2] != "validate" || got[len(got)-4] != "-beta" {
		t.Fatalf("%v", got)
	}
}

func TestRemoteAndOutdated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("itemcount") != "3" {
			t.Errorf("itemcount=%s", r.Form.Get("itemcount"))
		}
		w.Write([]byte(`{"response":{"publishedfiledetails":[
			{"publishedfileid":"111","result":1,"title":"A","time_updated":150},
			{"publishedfileid":"222","result":1,"title":"B","time_updated":200},
			{"publishedfileid":"333","result":9,"title":""}]}}`))
	}))
	defer srv.Close()
	remote, err := RemoteMods(srv.Client(), srv.URL, []string{"111", "222", "333"})
	if err != nil {
		t.Fatal(err)
	}
	up := OutdatedMods([]string{"111", "222", "333", "444"}, map[string]int64{"111": 100, "222": 200}, remote)
	if len(up) != 1 || up[0].ID != "111" {
		t.Fatalf("%+v", up)
	}
	up = OutdatedMods([]string{"222"}, map[string]int64{}, remote)
	if len(up) != 1 || !up[0].NotLoaded {
		t.Fatalf("%+v", up)
	}
}

func TestWorkshopArgs(t *testing.T) {
	got := WorkshopArgs("/d", "123456")
	if got[len(got)-3] != "108600" || got[len(got)-2] != "123456" {
		t.Fatalf("%v", got)
	}
}
