package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/json/option"
)

const (
	themesDataURL = "https://rautobot.github.io/themes-repo/data.json"
	themesPerPage = 5
)

type aliucordTheme struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Author   string `json:"author"`
	URL      string `json:"url"`
	RepoURL  string `json:"repoUrl"`
	Filename string `json:"filename"`
}

type themesCache struct {
	data   []aliucordTheme
	expiry int64
}

var (
	tCache      = make(map[string]*themesCache)
	tCacheMutex sync.Mutex
)

func init() {
	addCommand(&Command{
		CreateCommandData: api.CreateCommandData{
			Name:        "themes",
			Description: "Browse Aliucord themes",
			Options: []discord.CommandOption{
				&discord.StringOption{OptionName: "search", Description: "Search by theme name"},
				&discord.StringOption{OptionName: "author", Description: "Filter by author"},
				&discord.BooleanOption{OptionName: "send", Description: "Send publicly (default: private)"},
				&discord.IntegerOption{OptionName: "page", Description: "Page number", Min: option.NewInt(1)},
			},
		},
		Execute: themesCommand,
	})
}

func themesCommand(e *gateway.InteractionCreateEvent, d *discord.CommandInteraction) error {
	var search, author string
	sendPublic := false
	page := 1

	for _, opt := range d.Options {
		switch opt.Name {
		case "search":
			search = strings.TrimSpace(opt.String())
		case "author":
			author = strings.TrimSpace(opt.String())
		case "send":
			b, err := opt.BoolValue()
			if err == nil {
				sendPublic = b
			}
		case "page":
			p, err := opt.IntValue()
			if err == nil && p > 0 {
				page = int(p)
			}
		}
	}

	content, totalPages, comps, err := renderThemesPage(search, author, page)
	if err != nil {
		return replyErr(e, "rendering themes", err)
	}

	flags := discord.EphemeralMessage
	if sendPublic {
		flags = 0
	}
	err = s.RespondInteraction(e.ID, e.Token, api.InteractionResponse{
		Type: api.MessageInteractionWithSource,
		Data: &api.InteractionResponseData{
			Content:         option.NewNullableString(content),
			Flags:           flags,
			Components:      &comps,
			AllowedMentions: &api.AllowedMentions{},
		},
	})
	if err != nil {
		return err
	}

	if msg, err := s.InteractionResponse(e.AppID, e.Token); err == nil && msg != nil {
		paginationState[msg.ID] = &listQuery{Kind: "themes", Search: search, Author: author, Page: page, Total: totalPages}
	}
	return nil
}

// renderThemesPage builds content and components for the given filters and page
func renderThemesPage(search, author string, page int) (string, int, discord.ContainerComponents, error) {
	themes, err := fetchThemes()
	if err != nil {
		return "", 0, nil, err
	}
	filtered := filterThemes(themes, search, author)
	if len(filtered) == 0 {
		return "No themes found.", 1, discord.ContainerComponents{}, nil
	}

	totalPages := (len(filtered) + themesPerPage - 1) / themesPerPage
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * themesPerPage
	end := start + themesPerPage
	if end > len(filtered) {
		end = len(filtered)
	}
	pageThemes := filtered[start:end]

	var b strings.Builder
	if search != "" || author != "" {
		var parts []string
		if search != "" {
			parts = append(parts, fmt.Sprintf("\"%s\"", search))
		}
		if author != "" {
			parts = append(parts, fmt.Sprintf("by %s", author))
		}
		b.WriteString(fmt.Sprintf("**Themes %s** (%d found)\n\n", strings.Join(parts, " "), len(filtered)))
	} else {
		b.WriteString(fmt.Sprintf("**All Themes** (Page %d/%d)\n\n", page, totalPages))
	}

	for i, t := range pageThemes {
		b.WriteString(formatThemeLine(t))
		if i < len(pageThemes)-1 {
			b.WriteString("\n\n───────────────\n\n")
		}
	}
	b.WriteString("\n\n-# hold this message (not the links) to install")

	row := discord.ActionRowComponent{
		&discord.ButtonComponent{Label: "Prev", CustomID: "page:themes:prev", Style: discord.SecondaryButtonStyle(), Disabled: page <= 1},
		&discord.ButtonComponent{Label: "Next", CustomID: "page:themes:next", Style: discord.PrimaryButtonStyle(), Disabled: page >= totalPages},
	}
	comps := discord.ContainerComponents{&row}
	return b.String(), totalPages, comps, nil
}

func fetchThemes() ([]aliucordTheme, error) {
	tCacheMutex.Lock()
	defer tCacheMutex.Unlock()

	if dl, ok := tCache["all"]; ok && dl.expiry > time.Now().Unix() {
		return dl.data, nil
	}

	resp, err := http.Get(themesDataURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("themes http %d", resp.StatusCode)
	}

	var arr []aliucordTheme
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
		return nil, err
	}

	out := make([]aliucordTheme, 0, len(arr))
	for _, t := range arr {
		if strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.URL) == "" {
			continue
		}
		if strings.TrimSpace(t.Author) == "" {
			t.Author = "Unknown"
		}
		out = append(out, t)
	}

	tCache["all"] = &themesCache{
		data:   out,
		expiry: time.Now().Add(15 * time.Minute).Unix(),
	}

	return out, nil
}

func filterThemes(list []aliucordTheme, search, author string) []aliucordTheme {
	res := make([]aliucordTheme, 0, len(list))
	s := strings.ToLower(strings.TrimSpace(search))
	a := strings.ToLower(strings.TrimSpace(author))
	for _, t := range list {
		if a != "" && !strings.Contains(strings.ToLower(t.Author), a) {
			continue
		}
		if s != "" && !strings.Contains(strings.ToLower(t.Name), s) {
			continue
		}
		res = append(res, t)
	}
	return res
}

func formatThemeLine(t aliucordTheme) string {
	name := escapeMarkdown(t.Name)
	ver := escapeMarkdown(strings.TrimSpace(t.Version))
	auth := escapeMarkdown(t.Author)
	small := auth
	if ver != "" {
		small = fmt.Sprintf("v%s %s", ver, auth)
	}
	return fmt.Sprintf("[%s](<%s>)\n-# %s", name, t.URL, small)
}
