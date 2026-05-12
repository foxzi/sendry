package handlers

import (
	"fmt"
	"html"
	"strings"

	"github.com/foxzi/sendry/internal/web/blocks"
	"github.com/foxzi/sendry/internal/web/models"
)

func (h *Handlers) rebuildTemplateHTML(refs []models.TemplateBlockRef, opts WrapperOpts) (string, error) {
	if len(refs) == 0 {
		return "", nil
	}

	var sb strings.Builder
	for i, ref := range refs {
		b, err := h.blocks.GetByID(ref.BlockID)
		if err != nil {
			return "", fmt.Errorf("load block %s: %w", ref.BlockID, err)
		}
		if b == nil {
			continue
		}
		_ = i
		wrapped := WrapBlockShell(b.HTML, b.BorderRadius, b.PaddingV, b.PaddingH, b.Background)
		cond := strings.TrimSpace(ref.Condition)
		if cond != "" {
			sb.WriteString(fmt.Sprintf("{{if .%s}}\n", cond))
		}
		sb.WriteString(wrapped)
		sb.WriteString("\n")
		if i < len(refs)-1 && ref.GapHeight > 0 {
			sb.WriteString(spacerRow(ref.GapHeight, ref.GapColor))
			sb.WriteString("\n")
		}
		if cond != "" {
			sb.WriteString("{{end}}\n")
		}
	}
	return ApplyWrapper(sb.String(), opts)
}

func WrapBlockShell(inner string, radius, padV, padH int, background string) string {
	return WrapBlockShellCorners(inner, radius, padV, padH, background, 0, 0)
}

func WrapBlockShellCorners(inner string, radius, padV, padH int, background string, topInherit, bottomInherit int) string {
	bg := strings.TrimSpace(background)
	if bg == "" && (radius > 0 || topInherit > 0 || bottomInherit > 0) {
		bg = "#FEFFFE"
	}
	if radius <= 0 && padV <= 0 && padH <= 0 && bg == "" && topInherit <= 0 && bottomInherit <= 0 {
		return inner
	}
	if padV < 0 {
		padV = 0
	}
	if padH < 0 {
		padH = 0
	}
	tableStyles := []string{"width:100%", "border-collapse:separate"}
	if radius > 0 {
		tableStyles = append(tableStyles, fmt.Sprintf("border-radius:%dpx", radius), "overflow:hidden")
	} else if topInherit > 0 || bottomInherit > 0 {
		tl, tr, br, bl := topInherit, topInherit, bottomInherit, bottomInherit
		tableStyles = append(tableStyles, fmt.Sprintf("border-radius:%dpx %dpx %dpx %dpx", tl, tr, br, bl), "overflow:hidden")
	} else {
		tableStyles = append(tableStyles, "border-radius:0")
	}
	if bg != "" {
		tableStyles = append(tableStyles, "background-color:"+bg)
	}
	tableStyle := strings.Join(tableStyles, ";") + ";"
	contentTdStyle := fmt.Sprintf("padding:%dpx %dpx;", padV, padH)
	trimmed := strings.TrimLeft(inner, " \t\r\n")
	if strings.HasPrefix(strings.ToLower(trimmed), "<table") {
		return fmt.Sprintf(
			`<tr><td style="padding:0;"><table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="%s"><tr><td style="%s">%s</td></tr></table></td></tr>`,
			tableStyle, contentTdStyle, inner,
		)
	}
	return fmt.Sprintf(
		`<tr><td style="padding:0;"><table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="%s"><tr><td style="%s"><table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="border-collapse:collapse;width:100%%;">%s</table></td></tr></table></td></tr>`,
		tableStyle, contentTdStyle, inner,
	)
}

type WrapperOpts struct {
	Radius       int
	RadiusTop    int // overrides Radius for TL+TR when > 0
	RadiusBottom int // overrides Radius for BL+BR when > 0
	Transparent  bool
	Width        int
	PaddingV     int
	PaddingH     int
	PageBG       string
}

func ApplyWrapper(inner string, opts WrapperOpts) (string, error) {
	wrapper, err := blocks.GetWrapper()
	if err != nil {
		return "", fmt.Errorf("load wrapper: %w", err)
	}
	radius := opts.Radius
	if radius < 0 {
		radius = 0
	}
	width := opts.Width
	if width <= 0 {
		width = 600
	}
	padV := opts.PaddingV
	if padV < 0 {
		padV = 0
	}
	padH := opts.PaddingH
	if padH < 0 {
		padH = 0
	}
	pageBG := strings.TrimSpace(opts.PageBG)
	if pageBG == "" {
		pageBG = "#F5F5F5"
	}
	containerBG := "#FEFFFE"
	if opts.Transparent {
		containerBG = "transparent"
	}
	top := opts.RadiusTop
	if top <= 0 {
		top = radius
	}
	bottom := opts.RadiusBottom
	if bottom <= 0 {
		bottom = radius
	}
	wrapper = strings.ReplaceAll(wrapper, "{{CONTAINER_RADIUS}}", fmt.Sprintf("%dpx %dpx %dpx %dpx", top, top, bottom, bottom))
	wrapper = strings.ReplaceAll(wrapper, "{{CONTAINER_WIDTH}}", fmt.Sprintf("%d", width))
	wrapper = strings.ReplaceAll(wrapper, "{{CONTAINER_PADDING_V}}", fmt.Sprintf("%d", padV))
	wrapper = strings.ReplaceAll(wrapper, "{{CONTAINER_PADDING_H}}", fmt.Sprintf("%d", padH))
	wrapper = strings.ReplaceAll(wrapper, "{{CONTAINER_BG}}", containerBG)
	wrapper = strings.ReplaceAll(wrapper, "{{PAGE_BG}}", pageBG)
	const placeholder = "{{BLOCKS}}"
	if idx := strings.Index(wrapper, placeholder); idx != -1 {
		return wrapper[:idx] + inner + wrapper[idx+len(placeholder):], nil
	}
	return inner, nil
}

func spacerRow(height int, color string) string {
	c := strings.TrimSpace(color)
	bgAttr := ""
	bgInline := ""
	if c != "" {
		safe := html.EscapeString(c)
		bgAttr = ` bgcolor="` + safe + `"`
		bgInline = "background-color:" + safe + ";"
	}
	return fmt.Sprintf(
		`<tr><td height="%d"%s style="font-size:0;line-height:0;height:%dpx;%s">&nbsp;</td></tr>`,
		height, bgAttr, height, bgInline,
	)
}

func (h *Handlers) RebuildAllBlockTemplates(actorEmail string) (rebuilt, skipped, failed int, err error) {
	all, _, lerr := h.templates.List(models.TemplateListFilter{Limit: 100000})
	if lerr != nil {
		return 0, 0, 0, fmt.Errorf("list templates: %w", lerr)
	}
	for _, t := range all {
		if !t.UseBlocks {
			skipped++
			continue
		}
		refs, rerr := h.templates.GetBlockRefs(t.ID)
		if rerr != nil {
			h.logger.Error("rebuild-all: load refs", "template_id", t.ID, "error", rerr)
			failed++
			continue
		}
		if len(refs) == 0 {
			skipped++
			continue
		}
		full, ferr := h.templates.GetByID(t.ID)
		if ferr != nil || full == nil {
			h.logger.Error("rebuild-all: load template", "template_id", t.ID, "error", ferr)
			failed++
			continue
		}
		newHTML, herr := h.rebuildTemplateHTML(refs, WrapperOpts{
			Radius:       full.ContainerRadius,
			RadiusTop:    full.ContainerRadiusTop,
			RadiusBottom: full.ContainerRadiusBottom,
			Transparent:  full.ContainerTransparent,
			Width:        full.ContainerWidth,
			PaddingV:     full.ContainerPaddingV,
			PaddingH:     full.ContainerPaddingH,
			PageBG:       full.PageBackground,
		})
		if herr != nil {
			h.logger.Error("rebuild-all: assemble html", "template_id", t.ID, "error", herr)
			failed++
			continue
		}
		full.HTML = newHTML
		full.UseBlocks = true
		if uerr := h.templates.Update(full, "Auto-rebuild: wrapper update", actorEmail); uerr != nil {
			h.logger.Error("rebuild-all: update template", "template_id", t.ID, "error", uerr)
			failed++
			continue
		}
		rebuilt++
	}
	return rebuilt, skipped, failed, nil
}

func (h *Handlers) rebuildTemplatesUsingBlock(blockID, actorEmail string) (int, error) {
	templates, err := h.templates.GetTemplatesByBlockID(blockID)
	if err != nil {
		return 0, fmt.Errorf("list templates: %w", err)
	}
	count := 0
	for _, t := range templates {
		refs, err := h.templates.GetBlockRefs(t.ID)
		if err != nil {
			h.logger.Error("rebuild: load refs", "template_id", t.ID, "error", err)
			continue
		}
		if len(refs) == 0 {
			continue
		}
		full, ferr := h.templates.GetByID(t.ID)
		if ferr != nil || full == nil {
			h.logger.Error("rebuild: load template", "template_id", t.ID, "error", ferr)
			continue
		}
		newHTML, err := h.rebuildTemplateHTML(refs, WrapperOpts{
			Radius:       full.ContainerRadius,
			RadiusTop:    full.ContainerRadiusTop,
			RadiusBottom: full.ContainerRadiusBottom,
			Transparent:  full.ContainerTransparent,
			Width:        full.ContainerWidth,
			PaddingV:     full.ContainerPaddingV,
			PaddingH:     full.ContainerPaddingH,
			PageBG:       full.PageBackground,
		})
		if err != nil {
			h.logger.Error("rebuild: assemble html", "template_id", t.ID, "error", err)
			continue
		}
		full.HTML = newHTML
		full.UseBlocks = true
		if err := h.templates.Update(full, "Auto-rebuild after block change", actorEmail); err != nil {
			h.logger.Error("rebuild: update template", "template_id", t.ID, "error", err)
			continue
		}
		count++
	}
	return count, nil
}
