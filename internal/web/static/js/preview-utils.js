// Shared preview helpers for the email builder.
// Strips only the wrapper's dark-mode block from preview HTML so previews
// stay readable on a forced white background. Wrapper block is identified
// by the .force-page-bg selector; block/template-specific dark-mode CSS
// is preserved. Keep this in sync with internal/web/handlers/templates.go
// (stripDarkModeCSS) — both must match the wrapper CSS in
// internal/web/blocks/wrapper.html.
(function(global) {
    var darkBlockRe = /@media\s*\(prefers-color-scheme:\s*dark\)\s*\{[\s\S]*?\}\s*\}/g;
    function stripWrapperDarkModeCSS(html) {
        if (typeof html !== 'string' || html.indexOf('@media') === -1) return html;
        return html.replace(darkBlockRe, function(m) {
            return m.indexOf('.force-page-bg') !== -1 ? '' : m;
        });
    }
    global.SendryPreview = global.SendryPreview || {};
    global.SendryPreview.stripWrapperDarkModeCSS = stripWrapperDarkModeCSS;
})(window);
