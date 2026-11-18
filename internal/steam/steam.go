// Package steam knows how to ask SteamCMD and the Steam Web API whether the
// Project Zomboid server or its Workshop mods have updates.
package steam

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/GirelliDev/deadbolt/internal/vdf"
)

const (
	AppID         = "380870" // Project Zomboid Dedicated Server
	WorkshopAppID = "108600" // Project Zomboid (Workshop items live under the game id)
	detailsURL    = "https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/"
)

// Runner executes steamcmd with args and returns its stdout; replaceable in tests.
type Runner func(args ...string) (string, error)

// Installed reads the build id and branch of the installed server.
func Installed(installDir string) (build, branch string, err error) {
	b, err := os.ReadFile(filepath.Join(installDir, "steamapps", "appmanifest_"+AppID+".acf"))
	if err != nil {
		return "", "", err
	}
	n, err := vdf.Parse(string(b))
	if err != nil {
		return "", "", err
	}
	build, ok := n.String("AppState", "buildid")
	if !ok {
		return "", "", errors.New("buildid missing in appmanifest")
	}
	branch = "public"
	for _, sec := range []string{"UserConfig", "MountedConfig"} {
		if k, ok := n.String("AppState", sec, "BetaKey"); ok && k != "" {
			branch = k
			break
		}
	}
	return build, branch, nil
}

// Latest asks Steam for the newest build id of branch.
func Latest(run Runner, branch string) (string, error) {
	out, err := run("+login", "anonymous", "+app_info_update", "1", "+app_info_print", AppID, "+quit")
	if err != nil {
		return "", fmt.Errorf("steamcmd: %w", err)
	}
	i := strings.Index(out, `"`+AppID+`"`)
	if i < 0 {
		return "", errors.New("steamcmd: app info not found in output")
	}
	n, err := vdf.Parse(out[i:])
	if err != nil {
		return "", err
	}
	id, ok := n.String(AppID, "depots", "branches", branch, "buildid")
	if !ok {
		return "", fmt.Errorf("branch %q not found for app %s", branch, AppID)
	}
	return id, nil
}

// InstallArgs are the steamcmd arguments to install or update into dir.
func InstallArgs(dir, branch string, validate bool) []string {
	a := []string{"+force_install_dir", dir, "+login", "anonymous", "+app_update", AppID}
	if branch != "" && branch != "public" {
		a = append(a, "-beta", branch)
	}
	if validate {
		a = append(a, "validate")
	}
	return append(a, "+quit")
}

// LocalMods returns installed Workshop item id -> timeupdated (unix seconds).
func LocalMods(installDir string) (map[string]int64, error) {
	b, err := os.ReadFile(filepath.Join(installDir, "steamapps", "workshop", "appworkshop_"+WorkshopAppID+".acf"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]int64{}, nil
	}
	if err != nil {
		return nil, err
	}
	n, err := vdf.Parse(string(b))
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	items, _ := n.Map("AppWorkshop", "WorkshopItemsInstalled")
	for id := range items {
		if s, ok := items.String(id, "timeupdated"); ok {
			out[id], _ = strconv.ParseInt(s, 10, 64)
		}
	}
	return out, nil
}

type RemoteMod struct {
	ID      string
	Title   string
	Updated int64
	Missing bool // removed from the Workshop or hidden
}

// RemoteMods queries the public Steam Web API (no key needed) in batches.
func RemoteMods(c *http.Client, apiURL string, ids []string) (map[string]RemoteMod, error) {
	if apiURL == "" {
		apiURL = detailsURL
	}
	out := map[string]RemoteMod{}
	for start := 0; start < len(ids); start += 100 {
		end := min(start+100, len(ids))
		form := url.Values{"itemcount": {strconv.Itoa(end - start)}}
		for i, id := range ids[start:end] {
			form.Set(fmt.Sprintf("publishedfileids[%d]", i), id)
		}
		resp, err := c.PostForm(apiURL, form)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("steam web api: HTTP %d", resp.StatusCode)
		}
		var r struct {
			Response struct {
				Details []struct {
					ID      string `json:"publishedfileid"`
					Result  int    `json:"result"`
					Title   string `json:"title"`
					Updated int64  `json:"time_updated"`
				} `json:"publishedfiledetails"`
			} `json:"response"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			return nil, fmt.Errorf("steam web api: %w", err)
		}
		for _, d := range r.Response.Details {
			out[d.ID] = RemoteMod{ID: d.ID, Title: d.Title, Updated: d.Updated, Missing: d.Result != 1}
		}
	}
	return out, nil
}

type ModUpdate struct {
	ID, Title string
	NotLoaded bool // never downloaded yet
}

// OutdatedMods compares wanted ids against local and remote timestamps.
func OutdatedMods(wanted []string, local map[string]int64, remote map[string]RemoteMod) []ModUpdate {
	var out []ModUpdate
	for _, id := range wanted {
		r, ok := remote[id]
		if !ok || r.Missing {
			continue // can't judge; never block an update on a delisted item
		}
		l, have := local[id]
		if !have {
			out = append(out, ModUpdate{ID: id, Title: r.Title, NotLoaded: true})
		} else if r.Updated > l {
			out = append(out, ModUpdate{ID: id, Title: r.Title})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// SplitList splits a ;-separated ini value into trimmed non-empty items.
func SplitList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ";") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
