package controllers

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/rcy1314/echo-noise/pkg"
	"golang.org/x/net/html"
)

var renderedAttachmentLabelRegexp = regexp.MustCompile(`^(图片附件|视频附件|音频附件|文件附件)：[^\r\n\x{2028}\x{2029}]+$`)

// extractMessageImages follows rendered note content, including the image
// attachment links emitted by uploads, Markdown images, and HTML images.
func extractMessageImages(content string) []string {
	images := make([]string, 0)
	document, err := html.Parse(bytes.NewReader(pkg.MdToHTML([]byte(content))))
	if err != nil {
		return images
	}
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "template":
				return
			case "a":
				// Keep the same label rule as attachmentInfoFromRenderedAnchor.
				label := strings.TrimSpace(imageNodeText(node))
				if renderedAttachmentLabelRegexp.MatchString(label) {
					if href := imageNodeAttribute(node, "href"); href != "" {
						if strings.HasPrefix(label, "图片附件：") {
							images = append(images, href)
						}
						// All attachment labels replace the whole link in the note.
						return
					}
				}
			case "img":
				if source := imageNodeAttribute(node, "src"); source != "" {
					images = append(images, source)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return images
}

func imageNodeAttribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

func imageNodeText(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(imageNodeText(child))
	}
	return text.String()
}
