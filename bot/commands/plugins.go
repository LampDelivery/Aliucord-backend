package commands

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strings"

    "github.com/diamondburned/arikawa/v3/api"
    "github.com/diamondburned/arikawa/v3/discord"
    "github.com/diamondburned/arikawa/v3/gateway"
    "github.com/diamondburned/arikawa/v3/utils/json/option"
)

const (
    pluginsManifestURL = "https://plugins.aliucord.com/manifest.json"
    pluginsPerPage     = 5
)

type aliucordPlugin struct {
    Name        string   `json:"name"`
    Description string   `json:"description"`
    URL         string   `json:"url"`
    Version     string   `json:"version"`
    Authors     any      `json:"authors"`
    Changelog   string   `json:"changelog"`
}

func init() {
    addCommand(&Command{
        CreateCommandData: api.CreateCommandData{
            Name:        "plugins",
            Description: "Browse Aliucord plugins",
            Options: []discord.CommandOption{
                &discord.StringOption{OptionName: "search", Description: "Search by name or description"},
                &discord.StringOption{OptionName: "author", Description: "Filter by author"},
                &discord.BooleanOption{OptionName: "send", Description: "Send publicly (default: private)"},
                &discord.IntegerOption{OptionName: "page", Description: "Page number", Min: option.NewInt(1)},
            },
        },
        Execute: pluginsCommand,
    })
}

func pluginsCommand(e *gateway.InteractionCreateEvent, d *discord.CommandInteraction) error {
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

    content, totalPages, comps, err := renderPluginsPage(search, author, page)
    if err != nil {
        return replyErr(e, "rendering plugins", err)
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

    // Track pagination state for buttons
    if msg, err := s.InteractionResponse(e.AppID, e.Token); err == nil && msg != nil {
        paginationState[msg.ID] = &listQuery{Kind: "plugins", Search: search, Author: author, Page: page, Total: totalPages}
    }
    return nil
}

// renderPluginsPage builds the content and components for a given filter and page
func renderPluginsPage(search, author string, page int) (string, int, discord.ContainerComponents, error) {
    plugins, err := fetchPlugins()
    if err != nil {
        return "", 0, nil, err
    }
    filtered := filterPlugins(plugins, search, author)
    if len(filtered) == 0 {
        return "No plugins found.", 1, discord.ContainerComponents{}, nil
    }

    totalPages := (len(filtered) + pluginsPerPage - 1) / pluginsPerPage
    if page < 1 {
        page = 1
    }
    if page > totalPages {
        page = totalPages
    }
    start := (page - 1) * pluginsPerPage
    end := start + pluginsPerPage
    if end > len(filtered) {
        end = len(filtered)
    }
    pagePlugins := filtered[start:end]

    var b strings.Builder
    if search != "" || author != "" {
        var parts []string
        if search != "" {
            parts = append(parts, fmt.Sprintf("\"%s\"", search))
        }
        if author != "" {
            parts = append(parts, fmt.Sprintf("by %s", author))
        }
        b.WriteString(fmt.Sprintf("**Plugins %s** (%d found)\n\n", strings.Join(parts, " "), len(filtered)))
    } else {
        b.WriteString(fmt.Sprintf("**All Plugins** (Page %d/%d)\n\n", page, totalPages))
    }
    for i, p := range pagePlugins {
        b.WriteString(formatPluginLine(p))
        if i < len(pagePlugins)-1 {
            b.WriteString("\n\n")
        }
    }
    b.WriteString("\n\n-# hold this message (not the links) to install")

    // Build Prev/Next buttons
    row := discord.ActionRowComponent{
        &discord.ButtonComponent{Label: "Prev", CustomID: "page:plugins:prev", Style: discord.SecondaryButtonStyle(), Disabled: page <= 1},
        &discord.ButtonComponent{Label: "Next", CustomID: "page:plugins:next", Style: discord.PrimaryButtonStyle(), Disabled: page >= totalPages},
    }
    comps := discord.ContainerComponents{&row}
    return b.String(), totalPages, comps, nil
}

func fetchPlugins() ([]aliucordPlugin, error) {
    resp, err := http.Get(pluginsManifestURL)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("manifest http %d", resp.StatusCode)
    }

    var raw any
    if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
        return nil, err
    }

    // Manifest is expected to be an array
    arr, ok := raw.([]any)
    if !ok {
        return nil, fmt.Errorf("unexpected manifest format")
    }

    out := make([]aliucordPlugin, 0, len(arr))
    for _, v := range arr {
        m, _ := v.(map[string]any)
        if m == nil {
            continue
        }
        name, _ := m["name"].(string)
        url, _ := m["url"].(string)
        if name == "" || url == "" {
            continue
        }
        desc, _ := m["description"].(string)
        version, _ := m["version"].(string)
        authors := m["authors"]

        out = append(out, aliucordPlugin{
            Name:        name,
            Description: ternaryStr(desc != "", desc, "No description"),
            URL:         url,
            Version:     version,
            Authors:     authors,
            Changelog:   fmt.Sprint(m["changelog"]),
        })
    }
    return out, nil
}

func filterPlugins(list []aliucordPlugin, search, author string) []aliucordPlugin {
    res := make([]aliucordPlugin, 0, len(list))
    s := strings.ToLower(strings.TrimSpace(search))
    a := strings.ToLower(strings.TrimSpace(author))
    for _, p := range list {
        if a != "" {
            if !strings.Contains(strings.ToLower(extractAuthors(p.Authors)), a) {
                continue
            }
        }
        if s != "" {
            if !(strings.Contains(strings.ToLower(p.Name), s) || strings.Contains(strings.ToLower(p.Description), s)) {
                continue
            }
        }
        res = append(res, p)
    }
    return res
}

func formatPluginLine(p aliucordPlugin) string {
    name := escapeMarkdown(p.Name)
    desc := escapeMarkdown(p.Description)
    authors := escapeMarkdown(extractAuthors(p.Authors))
    return fmt.Sprintf("[%s](<%s>)\n%s - %s", name, p.URL, desc, authors)
}

func extractAuthors(v any) string {
    switch t := v.(type) {
    case string:
        if strings.TrimSpace(t) == "" {
            return "Unknown"
        }
        return t
    case []any:
        parts := make([]string, 0, len(t))
        for _, x := range t {
            parts = append(parts, fmt.Sprint(x))
        }
        return strings.Join(parts, ", ")
    default:
        return "Unknown"
    }
}

func escapeMarkdown(text string) string {
    r := strings.NewReplacer("*", "\\*", "_", "\\_", "~", "\\~", "`", "\\`", "[", "\\[", "]", "\\]", "(", "\\(", ")", "\\)")
    return r.Replace(text)
}

func ternaryStr(cond bool, a, b string) string {
    if cond {
        return a
    }
    return b
}
