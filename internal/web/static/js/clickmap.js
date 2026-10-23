// Click map: draws click-count badges over the sandboxed snapshot iframe.
// The iframe has no allow-scripts, but is same-origin, so the parent page can
// read its DOM and measure the [data-sl] elements.
(function () {
    var frame = document.getElementById('cm-frame');
    var overlay = document.getElementById('cm-overlay');
    var dataEl = document.getElementById('cm-data');
    if (!frame || !overlay || !dataEl) return;

    var links = [];
    try { links = JSON.parse(dataEl.textContent) || []; } catch (e) { links = []; }
    var byId = {};
    links.forEach(function (l) { byId[l.link_id] = l; });

    var mode = 'unique_clicks';

    function fitFrame(doc) {
        var h = Math.max(
            doc.documentElement ? doc.documentElement.scrollHeight : 0,
            doc.body ? doc.body.scrollHeight : 0
        );
        if (h > 0) frame.style.height = h + 'px';
    }

    function colour(share) {
        // Light blue to deep red by share of the maximum
        var hue = 210 - Math.round(210 * share);
        return 'hsl(' + hue + ', 75%, ' + (48 - Math.round(8 * share)) + '%)';
    }

    function draw() {
        var doc;
        try { doc = frame.contentDocument; } catch (e) { return; }
        if (!doc || !doc.body) return;
        fitFrame(doc);
        overlay.innerHTML = '';

        var max = 0;
        links.forEach(function (l) { if (l[mode] > max) max = l[mode]; });

        var seen = {};
        doc.querySelectorAll('[data-sl]').forEach(function (el) {
            var id = parseInt(el.getAttribute('data-sl'), 10);
            var info = byId[id];
            if (!info || seen[id]) return;
            seen[id] = true;
            var rect = el.getBoundingClientRect();
            if (rect.width === 0 && rect.height === 0) return;
            var count = info[mode] || 0;

            var badge = document.createElement('div');
            badge.className = 'cm-badge';
            badge.textContent = count;
            badge.style.left = Math.max(0, rect.left + rect.width - 12) + 'px';
            badge.style.top = Math.max(0, rect.top - 9) + 'px';
            badge.style.background = count > 0 && max > 0 ? colour(count / max) : '#9ca3af';
            badge.title = '#' + id + ' ' + (info.label || '') + '\n' + (info.url || '');
            overlay.appendChild(badge);
        });
    }

    frame.addEventListener('load', draw);
    if (frame.contentDocument && frame.contentDocument.readyState === 'complete') draw();
    window.addEventListener('resize', draw);

    document.querySelectorAll('#cm-mode [data-mode]').forEach(function (btn) {
        btn.addEventListener('click', function () {
            mode = btn.getAttribute('data-mode');
            document.querySelectorAll('#cm-mode [data-mode]').forEach(function (b) {
                b.classList.toggle('active', b === btn);
            });
            draw();
        });
    });
})();
