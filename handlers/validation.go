package handlers

import (
	"fmt"
	htmlstd "html"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const maxStoryHTMLBytes = 16000

var backgroundColorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validateAndSanitizeStoryUpdate(update *StoryUpdate) error {
	if len(update.Story) > maxStoryHTMLBytes {
		return fmt.Errorf("story exceeds %d bytes", maxStoryHTMLBytes)
	}
	story, err := sanitizeStoryHTML(update.Story)
	if err != nil {
		return err
	}
	if strings.TrimSpace(story) == "" {
		return fmt.Errorf("story_update.story is required")
	}
	if update.BackgroundColor != "" && !backgroundColorRegex.MatchString(update.BackgroundColor) {
		return fmt.Errorf("background_color must be a six-digit hex color")
	}
	update.Story = story
	return nil
}

func sanitizeStoryHTML(content string) (string, error) {
	contextNode := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(content), contextNode)
	if err != nil {
		return "", fmt.Errorf("parse story HTML: %w", err)
	}

	var sanitized strings.Builder
	for _, node := range nodes {
		renderSanitizedNode(&sanitized, node)
	}
	return sanitized.String(), nil
}

func renderSanitizedNode(output *strings.Builder, node *html.Node) {
	switch node.Type {
	case html.TextNode:
		output.WriteString(htmlstd.EscapeString(node.Data))
	case html.ElementNode:
		tag := strings.ToLower(node.Data)
		switch tag {
		case "script", "style", "iframe", "object", "embed", "svg", "math":
			return
		case "br":
			output.WriteString("<br>")
			return
		case "strong", "em":
			output.WriteByte('<')
			output.WriteString(tag)
			output.WriteByte('>')
			renderSanitizedChildren(output, node)
			output.WriteString("</")
			output.WriteString(tag)
			output.WriteByte('>')
			return
		case "span":
			if opening, allowed := sanitizedSpanOpening(node); allowed {
				output.WriteString(opening)
				renderSanitizedChildren(output, node)
				output.WriteString("</span>")
				return
			}
		}
		renderSanitizedChildren(output, node)
	}
}

func renderSanitizedChildren(output *strings.Builder, node *html.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderSanitizedNode(output, child)
	}
}

func sanitizedSpanOpening(node *html.Node) (string, bool) {
	className := ""
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, "class") {
			className = strings.Join(strings.Fields(attribute.Val), " ")
			break
		}
	}

	switch className {
	case "item-added", "item-removed", "tooltiptext":
		return `<span class="` + className + `">`, true
	case "proper-noun tooltip", "tooltip proper-noun":
		return `<span class="proper-noun tooltip" tabindex="0">`, true
	default:
		return "", false
	}
}
