// Delivery error donuts for the API sends page.
// Reads JSON from #delivery-errors-data and renders two panels into #delivery-errors-panels.
(function() {
    'use strict';

    var SVG_NS = 'http://www.w3.org/2000/svg';
    var SIZE = 132, CX = 66, CY = 66, R = 56, STROKE = 16, GAP = 2;
    var CIRC = 2 * Math.PI * R;

    // Fixed order; identity colour follows the type, never its rank.
    var TYPES = [
        { type: 'address_not_found', color: 'var(--viz-1)' },
        { type: 'temp_unavailable', color: 'var(--viz-2)' },
        { type: 'mailbox_full', color: 'var(--viz-3)' },
        { type: 'domain_unreachable', color: 'var(--viz-4)' },
        { type: 'rejected', color: 'var(--viz-5)' },
        { type: 'technical', color: 'var(--viz-6)' },
        { type: 'blocked', color: 'var(--viz-7)' },
        { type: 'spam', color: 'var(--viz-8)' },
        { type: 'unknown', color: 'var(--viz-neutral)' }
    ];

    var PANELS = [
        { field: 'Failed', status: 'failed', titleKey: 'cmp.err_failed_title', titleEn: 'Not delivered',
          descKey: 'cmp.err_failed_desc', descEn: 'The recipient server refused the message for good. The MTA will not try again.' },
        { field: 'Retrying', status: 'queued', titleKey: 'cmp.err_retry_title', titleEn: 'Queued after an error',
          descKey: 'cmp.err_retry_desc', descEn: 'A delivery attempt got a temporary refusal. The MTA keeps the message in its queue and will try again.' }
    ];

    var EN_LABELS = {
        address_not_found: 'Address does not exist',
        mailbox_full: 'Mailbox full',
        temp_unavailable: 'Temporarily unavailable',
        domain_unreachable: 'Domain unreachable',
        rejected: 'Rejected by server',
        spam: 'Rejected as spam',
        blocked: 'Blocked',
        technical: 'Temporary technical error',
        unknown: 'Unknown error'
    };

    function tr(key, fallback) {
        if (typeof I18n === 'undefined') return fallback;
        var v = I18n.t(key);
        return v === key ? fallback : v;
    }

    function el(tag, cls, text) {
        var e = document.createElement(tag);
        if (cls) e.className = cls;
        if (text !== undefined) e.textContent = text;
        return e;
    }

    function svgEl(tag, attrs) {
        var e = document.createElementNS(SVG_NS, tag);
        for (var k in attrs) e.setAttribute(k, attrs[k]);
        return e;
    }

    function href(status, type) {
        return '/sends?status=' + status + '&error_type=' + encodeURIComponent(type) + '#results';
    }

    // Whole percents summing to 100; the biggest slice absorbs rounding.
    function percents(items, total) {
        var sum = 0, big = 0, i;
        for (i = 0; i < items.length; i++) {
            items[i].pct = Math.round(items[i].count * 100 / total);
            sum += items[i].pct;
            if (items[i].count > items[big].count) big = i;
        }
        items[big].pct += 100 - sum;
    }

    function setHighlight(panel, type) {
        panel.classList.toggle('has-hl', !!type);
        panel.querySelectorAll('[data-type]').forEach(function(n) {
            n.classList.toggle('is-on', !!type && n.getAttribute('data-type') === type);
        });
    }

    function bindHighlight(panel, node) {
        var type = node.getAttribute('data-type');
        node.addEventListener('mouseenter', function() { setHighlight(panel, type); });
        node.addEventListener('mouseleave', function() { setHighlight(panel, null); });
        node.addEventListener('focusin', function() { setHighlight(panel, type); });
        node.addEventListener('focusout', function() { setHighlight(panel, null); });
    }

    function buildPanel(data, def) {
        var items = [], total = 0;
        TYPES.forEach(function(ty) {
            var row = null;
            for (var i = 0; i < data.length; i++) if (data[i].Type === ty.type) row = data[i];
            var count = row ? (row[def.field] || 0) : 0;
            if (count > 0) {
                items.push({ type: ty.type, color: ty.color, count: count });
                total += count;
            }
        });

        var panel = el('div', 'err-panel');
        panel.appendChild(el('div', 'err-panel-title', tr(def.titleKey, def.titleEn)));
        panel.appendChild(el('p', 'err-panel-desc', tr(def.descKey, def.descEn)));
        var body = el('div', 'err-body');
        panel.appendChild(body);

        var donut = el('div', 'err-donut');
        var svg = svgEl('svg', { width: SIZE, height: SIZE, viewBox: '0 0 ' + SIZE + ' ' + SIZE, 'aria-hidden': total ? 'false' : 'true' });
        // Muted track under the segments (and the only ring when empty).
        svg.appendChild(svgEl('circle', { cx: CX, cy: CY, r: R, fill: 'none', 'stroke-width': STROKE, 'class': 'err-track' }));

        if (total > 0) {
            percents(items, total);
            var offset = 0;
            items.forEach(function(it) {
                var len = CIRC * it.count / total;
                var dash = items.length > 1 ? Math.max(len - GAP, 0.01) : CIRC;
                var a = svgEl('a', { href: href(def.status, it.type), 'data-type': it.type, 'class': 'err-seg' });
                var title = document.createElementNS(SVG_NS, 'title');
                title.textContent = tr('error_type.' + it.type, EN_LABELS[it.type]) + ': ' + it.count + ' (' + it.pct + '%)';
                a.appendChild(title);
                a.appendChild(svgEl('circle', {
                    cx: CX, cy: CY, r: R, fill: 'none',
                    stroke: it.color, 'stroke-width': STROKE,
                    'stroke-dasharray': dash.toFixed(3) + ' ' + (CIRC - dash).toFixed(3),
                    'stroke-dashoffset': (-offset).toFixed(3),
                    transform: 'rotate(-90 ' + CX + ' ' + CY + ')'
                }));
                svg.appendChild(a);
                bindHighlight(panel, a);
                offset += len;
            });
        }
        donut.appendChild(svg);

        var center = el('div', 'err-center');
        center.appendChild(el('div', 'err-total', String(total)));
        center.appendChild(el('div', 'err-total-label', tr('cmp.err_messages', 'messages')));
        donut.appendChild(center);
        body.appendChild(donut);

        if (total === 0) {
            body.appendChild(el('div', 'err-none', tr('cmp.err_none', 'No messages')));
            return panel;
        }

        var ul = el('ul', 'err-legend');
        items.slice().sort(function(a, b) { return b.count - a.count; }).forEach(function(it) {
            var li = el('li', 'err-row');
            li.setAttribute('data-type', it.type);
            var sw = el('span', 'err-swatch');
            sw.style.background = it.color;
            li.appendChild(sw);
            var label = el('a', 'err-label', EN_LABELS[it.type]);
            label.setAttribute('href', href(def.status, it.type));
            label.setAttribute('data-i18n', 'error_type.' + it.type);
            label.textContent = tr('error_type.' + it.type, EN_LABELS[it.type]);
            li.appendChild(label);
            li.appendChild(el('span', 'err-count', String(it.count)));
            li.appendChild(el('span', 'err-pct', it.pct + '%'));
            bindHighlight(panel, li);
            ul.appendChild(li);
        });
        body.appendChild(ul);
        return panel;
    }

    function render() {
        var src = document.getElementById('delivery-errors-data');
        var host = document.getElementById('delivery-errors-panels');
        if (!src || !host) return;
        var data;
        try { data = JSON.parse(src.textContent); } catch (e) { return; }
        if (!Array.isArray(data)) return;
        host.textContent = '';
        PANELS.forEach(function(def) { host.appendChild(buildPanel(data, def)); });
        host.classList.add('is-ready');
    }

    document.addEventListener('DOMContentLoaded', function() {
        render();
        document.querySelectorAll('.lang-btn').forEach(function(btn) {
            // i18n.js registers its own click handler later; wait for it to switch language.
            btn.addEventListener('click', function() { setTimeout(render, 0); });
        });
    });
})();
