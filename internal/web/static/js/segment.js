// Segments of a recipient list (see models/segment.go): the fields and operators of a condition
// and the text that describes a segment. Elements with data-segment="<JSON>" get that text
// (the launch page, the scheduled card of a campaign). Translations: cf.sg_* in i18n/campaign-flow.js.
(function () {
    'use strict';

    var ACTIVITY = ['last_days', 'not_last_days', 'after', 'before', 'never'];
    // Operators of each field, in the order the form offers them
    var FIELDS = {
        added: ['last_days', 'after', 'before'],
        opened: ACTIVITY,
        clicked: ACTIVITY,
        received: ACTIVITY,
        tag: ['has', 'not_has'],
        'var': ['eq', 'ne', 'contains', 'gt', 'lt', 'set', 'not_set'],
        email: ['contains', 'not_contains']
    };
    var ORDER = ['added', 'opened', 'clicked', 'received', 'tag', 'var', 'email'];

    function t(key, params) { return window.I18n ? I18n.t(key, params) : key; }

    // What the value of an operator is: days, date, number, text, or none
    function valueKind(op) {
        if (op === 'last_days' || op === 'not_last_days') return 'days';
        if (op === 'after' || op === 'before') return 'date';
        if (op === 'never' || op === 'set' || op === 'not_set') return 'none';
        if (op === 'gt' || op === 'lt') return 'number';
        return 'text';
    }

    function parse(raw) {
        if (!raw) return null;
        if (typeof raw === 'object') return raw;
        try {
            var s = JSON.parse(raw);
            return s && Array.isArray(s.conditions) && s.conditions.length ? s : null;
        } catch (e) { return null; }
    }

    // A date value (YYYY-MM-DD) the way the language writes dates
    function dateText(v) {
        var m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(v || '');
        if (!m) return v || '';
        var ru = window.I18n && I18n.getLang() === 'ru';
        return ru ? m[3] + '.' + m[2] + '.' + m[1] : v;
    }

    function describeCondition(c) {
        // "Tag «vip» is set" reads worse than a phrase of its own
        if (c.field === 'tag') return t('cf.sg_d_tag_' + c.op, { v: c.value || '' });
        var label = t('cf.sg_f_' + c.field);
        if (c.field === 'var') label += ' ' + (c.key || '');
        var value = valueKind(c.op) === 'date' ? dateText(c.value) : (c.value || '');
        return label + ' ' + t('cf.sg_d_' + c.op, { v: value });
    }

    // "Tag has «vip» and Opened emails in the last 30 days"
    function describe(raw) {
        var seg = parse(raw);
        if (!seg) return '';
        var glue = ' ' + t(seg.match === 'any' ? 'cf.sg_or' : 'cf.sg_and') + ' ';
        return seg.conditions.map(describeCondition).join(glue);
    }

    function fill() {
        Array.prototype.forEach.call(document.querySelectorAll('[data-segment]'), function (e) {
            var text = describe(e.getAttribute('data-segment'));
            if (text) e.textContent = text;
        });
    }

    window.SendrySegment = { FIELDS: FIELDS, ORDER: ORDER, valueKind: valueKind, parse: parse, describe: describe };

    function init() {
        fill();
        Array.prototype.forEach.call(document.querySelectorAll('.lang-btn'), function (b) {
            b.addEventListener('click', function () { setTimeout(fill, 0); });
        });
    }
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
    else init();
})();
