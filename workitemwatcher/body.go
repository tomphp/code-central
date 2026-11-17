// An Incoming Issue's body: a line with the work item's type and State
// linking to it, then its rich text fields, which Azure DevOps keeps as
// HTML, converted to Markdown.

package workitemwatcher

import (
	"fmt"
	"strings"

	"github.com/JohannesKaufmann/dom"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"

	"github.com/tomoram/code-central/azdo"
)

// body is the Incoming Issue's body for work item w, whose web link is
// link: the line `**{Type}** · {State} · [View in Azure Boards]({link})`,
// then the Description, then a Repro Steps and an Acceptance Criteria
// section, each only when its field has something in it.
func body(w azdo.WorkItem, link string) string {
	parts := []string{fmt.Sprintf("**%s** · %s · [View in Azure Boards](%s)", w.Fields.Type, w.Fields.State, link)}
	if d := markdown(w.Fields.Description); d != "" {
		parts = append(parts, d)
	}
	for _, s := range []struct{ heading, html string }{
		{"Repro Steps", w.Fields.ReproSteps},
		{"Acceptance Criteria", w.Fields.AcceptanceCriteria},
	} {
		if md := markdown(s.html); md != "" {
			parts = append(parts, "## "+s.heading+"\n\n"+md)
		}
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// markdown is a rich text field's HTML as Markdown, trimmed; empty when
// it shows nothing. HTML that can't be converted is kept as it is.
func markdown(h string) string {
	if strings.TrimSpace(h) == "" {
		return ""
	}
	md, err := toMarkdown.ConvertString(h)
	if err != nil {
		return strings.TrimSpace(h)
	}
	return strings.TrimSpace(md)
}

// toMarkdown converts HTML as GitHub-flavoured Markdown, with an image as
// a link to it, so nothing of Azure DevOps' is shown, and so fetched,
// without the human's say.
var toMarkdown = func() *converter.Converter {
	c := converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(), commonmark.NewCommonmarkPlugin(), table.NewTablePlugin(), strikethrough.NewStrikethroughPlugin()))
	c.Register.RendererFor("img", converter.TagTypeInline, imageAsLink, converter.PriorityEarly)
	return c
}()

// imageAsLink renders an img as a link to its source, named by its alt
// text, or "image".
func imageAsLink(ctx converter.Context, w converter.Writer, n *html.Node) converter.RenderStatus {
	src := strings.TrimSpace(dom.GetAttributeOr(n, "src", ""))
	if src == "" {
		return converter.RenderTryNext
	}
	alt := strings.Join(strings.Fields(dom.GetAttributeOr(n, "alt", "")), " ")
	if alt == "" {
		alt = "image"
	}
	alt = strings.NewReplacer("[", `\[`, "]", `\]`).Replace(alt)
	w.WriteString("[" + alt + "](" + ctx.AssembleAbsoluteURL(ctx, "img", src) + ")")
	return converter.RenderSuccess
}
