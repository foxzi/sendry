// Campaign rows: create (GET /campaigns/new), edit (GET /campaigns/{id}) and send (GET /campaigns/{id}/send).
// One generic row controller plus small per-row functions (summary, validation, fields).
// Config (window.CF_CONFIG) comes from the view; templates, lists and servers are rendered into the DOM.
// Loaded from the page body, before the layout scripts, so everything waits for DOMContentLoaded.
(function () {
    'use strict';

    var cfg = window.CF_CONFIG || {};
    var senderDomains = cfg.senderDomains || {};
    var EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    var THUMB_WIDTH = 600; // template thumbnails are rendered at this width and scaled down
    var LETTER = ['from', 'content', 'vars', 'track', 'about']; // rows of the "Letter" card
    var OPTIONAL = ['vars', 'track', 'about']; // optional rows in create mode

    function t(key, params) { return window.I18n ? I18n.t(key, params) : key; }
    function $(id) { return document.getElementById(id); }
    function all(root, sel) { return Array.prototype.slice.call(root.querySelectorAll(sel)); }

    // Small DOM builder: el('span', 'cf-sub', 'text')
    function el(tag, cls, text) {
        var e = document.createElement(tag);
        if (cls) e.className = cls;
        if (text !== undefined && text !== null) e.textContent = text;
        return e;
    }

    function init() {
        var root = $('cf');
        if (!root) return;

        var mode = cfg.mode || root.getAttribute('data-mode') || 'create';
        var campaignId = cfg.campaignId || root.getAttribute('data-campaign-id') || '';
        var form = $('cf-form');
        var rows = all(root, '.cf-row');
        var rowMap = {};
        rows.forEach(function (r) { rowMap[r.getAttribute('data-row')] = r; });

        // Step rows are walked one by one; the rest of the rows in edit/send are saved on their own
        var stepNames = mode === 'create' ? ['from', 'content', 'to', 'check', 'when']
            : mode === 'send' ? ['to', 'check', 'when'] : [];
        var steps = stepNames.map(function (n) { return rowMap[n]; }).filter(Boolean);
        var cur = 0; // index of the current step; steps.length means all done
        var open = null; // row whose fields are open
        var built = {}; // rows whose summary has been built (rebuilt on language switch)
        var testSentTo = ''; // set after a successful test email, shown in the "check" summary
        var submitting = false;

        function rowName(r) { return r.getAttribute('data-row'); }
        function isStep(r) { return steps.indexOf(r) !== -1; }
        // Rows saved on their own through fetch (all of them in edit, the "Letter" card in send)
        function isEditRow(r) {
            return mode === 'edit' || (mode === 'send' && LETTER.indexOf(rowName(r)) !== -1);
        }
        function val(name) {
            var e = root.querySelector('[name="' + name + '"]');
            return e && e.value ? e.value : '';
        }
        function trimmed(id) { var e = $(id); return e ? e.value.trim() : ''; }

        // ---------- errors ----------

        function errBox(name) { return root.querySelector('small.cf-err[data-err="' + name + '"]'); }

        // Message under a field (or the row's general error when the markup has no place for it)
        function setErr(name, key) {
            var box = errBox(name);
            if (!box && name === 'template_id_b') box = errBox('template_id');
            var f = root.querySelector('[name="' + name + '"]');
            if (f && f.type !== 'hidden' && f.type !== 'radio') {
                if (key) f.setAttribute('aria-invalid', 'true'); else f.removeAttribute('aria-invalid');
            }
            if (box) {
                box.textContent = key ? t(key) : '';
                box.hidden = !key;
                return;
            }
            if (!key) return;
            var row = f ? f.closest('.cf-row') : null;
            if (row) showRowError(row, t(key));
        }

        function showRowError(row, text) {
            var p = row.querySelector('p.cf-error');
            if (!p) return;
            p.textContent = text || '';
            p.hidden = !text;
        }

        function clearRowErrors(row) {
            all(row, 'small.cf-err').forEach(function (b) { b.hidden = true; b.textContent = ''; });
            all(row, '[aria-invalid]').forEach(function (f) { f.removeAttribute('aria-invalid'); });
            showRowError(row, '');
        }

        function focusFirstError(row) {
            var box = row.querySelector('small.cf-err:not([hidden])');
            if (!box) return;
            var f = root.querySelector('[name="' + box.getAttribute('data-err') + '"]');
            var details = box.closest('details');
            if (details) details.open = true;
            if (f && f.focus && f.type !== 'hidden') f.focus();
            else box.scrollIntoView({ block: 'center' });
        }

        // A field's message goes away as soon as the user touches it
        function clearOnTouch(e) {
            var n = e.target && e.target.name;
            if (n) setErr(n, '');
        }
        root.addEventListener('input', clearOnTouch);
        root.addEventListener('change', clearOnTouch);

        // ---------- templates and gallery (row "content") ----------

        var gal = $('cf-gal');
        var tplA = $('template_id');
        var tplB = $('template_id_b');
        var subA = $('subject');
        var subB = $('subject_b');
        var abFlag = $('ab_enabled');
        var splitEl = $('split_a');
        var curVer = 'a'; // version shown in the content row
        var folderFilter = '';

        function tplCard(id) {
            if (!gal || !id) return null;
            var found = null;
            all(gal, '.cf-tpl').forEach(function (c) { if (c.getAttribute('data-id') === id) found = c; });
            return found;
        }

        function tplInfo(id) {
            var c = tplCard(id);
            if (!c) return null;
            return { id: id, name: c.getAttribute('data-name') || '', subject: c.getAttribute('data-subject') || '', unsub: c.getAttribute('data-unsub') === '1' };
        }

        function abOn() { return !!abFlag && !abFlag.disabled; }
        function verInputs(v) { return v === 'b' ? { tpl: tplB, sub: subB } : { tpl: tplA, sub: subA }; }
        function verLabel(v) { return v === 'b' ? 'B' : 'A'; }

        // A subject the user has typed is not overwritten when the template changes
        function markTouched(input) {
            var card = tplCard((input === subB ? tplB : tplA).value);
            var custom = input.value !== '' && !(card && input.value === (card.getAttribute('data-subject') || ''));
            input.dataset.touched = custom ? '1' : '';
        }
        [subA, subB].forEach(function (inp) {
            if (!inp) return;
            markTouched(inp); // server-side value: custom unless it equals the template's subject
            inp.addEventListener('input', function () { inp.dataset.touched = inp.value ? '1' : ''; });
        });

        function setSelected(id) {
            if (!gal) return;
            all(gal, '.cf-tpl').forEach(function (c) {
                var on = !!id && c.getAttribute('data-id') === id;
                c.classList.toggle('sel', on);
                c.setAttribute('aria-pressed', on ? 'true' : 'false');
            });
        }

        function selectTemplate(id) {
            var v = verInputs(curVer);
            v.tpl.value = id;
            setSelected(id);
            var info = tplInfo(id);
            if (info && !v.sub.dataset.touched) v.sub.value = info.subject;
            setErr('template_id', '');
            renderTitle();
        }

        if (gal) {
            gal.addEventListener('click', function (e) {
                var card = e.target.closest('.cf-tpl');
                if (card) selectTemplate(card.getAttribute('data-id'));
            });
        }

        // Show version v: tabs, subject field, gallery selection, labels
        function showVer(v) {
            var on = abOn();
            curVer = on ? v : 'a';
            all(root, '.cf-vtabs button[data-ver]').forEach(function (b) {
                var is = b.getAttribute('data-ver') === curVer;
                b.classList.toggle('on', is);
                b.setAttribute('aria-pressed', is ? 'true' : 'false');
            });
            all(root, '[data-ver-field]').forEach(function (f) {
                f.hidden = f.getAttribute('data-ver-field') !== curVer;
            });
            var L = verLabel(curVer);
            var subLbl = $('cf-subject-label');
            var subBLbl = $('cf-subject-b-label');
            if (subLbl) subLbl.textContent = on ? t('cf.subject_ver', { v: 'A' }) : t('cf.subject');
            if (subBLbl) subBLbl.textContent = t('cf.subject_ver', { v: 'B' });
            var galLbl = $('cf-tpl-label');
            if (galLbl) galLbl.textContent = on ? t('cf.template_ver', { v: L }) : t('cf.template');
            setSelected(verInputs(curVer).tpl.value);
            renderSplit();
        }

        function renderSplit() {
            var lbl = $('cf-split-label');
            if (!lbl || !splitEl) return;
            var a = parseInt(splitEl.value, 10) || 50;
            lbl.textContent = 'A ' + a + '% / B ' + (100 - a) + '%';
        }
        if (splitEl) splitEl.addEventListener('input', renderSplit);

        // Repaint everything that depends on "is there a version B" from the fields' state
        function renderAB() {
            var on = abOn();
            all(root, '.cf-vtabs, .cf-ab-help, .cf-split').forEach(function (b) { b.hidden = !on; });
            // With B the versions are removed by the crosses on their tabs
            var link = root.querySelector('[data-act="ab"]');
            if (link) { link.textContent = t('cf.ab_add'); link.hidden = on; }
            showVer(curVer);
        }

        // Cross on a version tab: without A, B becomes A
        function deleteVersion(v) {
            if (v === 'a' && tplA && tplB) {
                tplA.value = tplB.value;
                subA.value = subB.value;
                subA.dataset.touched = subB.dataset.touched || '';
                setErr('template_id', '');
            }
            setAB(false);
            var tab = root.querySelector('.cf-vtabs button[data-ver="a"]');
            var link = root.querySelector('[data-act="ab"]');
            if (link) link.focus();
            else if (tab) tab.focus();
        }

        function setAB(on) {
            [abFlag, tplB, subB, splitEl].forEach(function (f) { if (f) f.disabled = !on; });
            if (!on) {
                if (tplB) tplB.value = '';
                if (subB) { subB.value = ''; subB.dataset.touched = ''; }
                // a version B added later starts from an even split
                if (splitEl) splitEl.value = 50;
                setErr('template_id_b', '');
                curVer = 'a';
            } else {
                curVer = 'b';
            }
            renderAB();
            renderTitle();
        }

        // Gallery filter: search by name plus a folder chip
        function filterGallery() {
            if (!gal) return;
            var search = $('cf-tpl-search');
            var q = ((search && search.value) || '').trim().toLowerCase();
            var shown = 0;
            all(gal, '.cf-tpl').forEach(function (c) {
                var okName = !q || (c.getAttribute('data-name') || '').toLowerCase().indexOf(q) !== -1;
                var okFolder = !folderFilter || c.getAttribute('data-folder') === folderFilter;
                c.hidden = !(okName && okFolder);
                if (!c.hidden) shown++;
            });
            var empty = $('cf-gal-empty');
            if (empty) empty.hidden = shown > 0;
            updateScale();
        }
        var searchEl = $('cf-tpl-search');
        if (searchEl) searchEl.addEventListener('input', filterGallery);
        all(root, '.cf-chips').forEach(function (chips) {
            chips.addEventListener('click', function (e) {
                var chip = e.target.closest('.cf-chip');
                if (!chip) return;
                folderFilter = chip.getAttribute('data-folder') || '';
                all(chips, '.cf-chip').forEach(function (c) { c.classList.toggle('on', c === chip); });
                filterGallery();
            });
        });

        // Thumbnails are 600px wide iframes; scale them to the card width
        function updateScale() {
            all(root, '.cf-gal').forEach(function (g) {
                var th = g.querySelector('.cf-tpl:not([hidden]) .cf-thumb');
                if (!th || !th.clientWidth) return;
                g.style.setProperty('--cf-scale', String(th.clientWidth / THUMB_WIDTH));
            });
        }
        window.addEventListener('resize', updateScale);
        if (window.ResizeObserver) {
            all(root, '.cf-gal').forEach(function (g) { new ResizeObserver(updateScale).observe(g); });
        }

        if (root.querySelector('[data-act="ab"]')) {
            // Version B is on when the server rendered one
            var hasB = !!(tplB && tplB.value);
            [abFlag, tplB, subB, splitEl].forEach(function (f) { if (f) f.disabled = !hasB; });
            renderAB();
        }

        // ---------- variables (row "vars") ----------

        var kv = $('cf-kv');
        var jsonEl = $('cf-vars-json');
        var jsonMode = false;

        function kvRow(k, v) {
            var r = el('div', 'cf-kv-row');
            var ki = el('input', 'input cf-kv-key');
            ki.type = 'text';
            ki.value = k || '';
            ki.placeholder = t('cf.kv_key');
            ki.setAttribute('aria-label', t('cf.kv_key'));
            var vi = el('input', 'input cf-kv-val');
            vi.type = 'text';
            vi.value = v || '';
            vi.placeholder = t('cf.kv_val');
            vi.setAttribute('aria-label', t('cf.kv_val'));
            var del = el('button', 'cf-kv-del', '×');
            del.type = 'button';
            del.setAttribute('data-act', 'kv-del');
            del.setAttribute('aria-label', t('cf.kv_del'));
            r.appendChild(ki);
            r.appendChild(vi);
            r.appendChild(del);
            return r;
        }

        // Pairs from the list editor, empty keys skipped
        function kvPairs() {
            var out = [];
            if (!kv) return out;
            all(kv, '.cf-kv-row').forEach(function (r) {
                var k = r.querySelector('.cf-kv-key').value.trim();
                if (k) out.push([k, r.querySelector('.cf-kv-val').value]);
            });
            return out;
        }

        function setKv(pairs) {
            if (!kv) return;
            kv.textContent = '';
            (pairs.length ? pairs : [['', '']]).forEach(function (p) { kv.appendChild(kvRow(p[0], p[1])); });
        }

        function pairsToObject(pairs) {
            var o = {};
            pairs.forEach(function (p) { o[p[0]] = p[1]; });
            return o;
        }

        // Parse the JSON textarea into pairs; values are made strings. Returns null when invalid.
        function parseJsonPairs() {
            var txt = jsonEl.value.trim();
            if (!txt) return [];
            var obj;
            try { obj = JSON.parse(txt); } catch (e) { return null; }
            if (!obj || typeof obj !== 'object' || Array.isArray(obj)) return null;
            var out = [];
            for (var k in obj) {
                if (!Object.prototype.hasOwnProperty.call(obj, k)) continue;
                var v = obj[k];
                if (v !== null && typeof v === 'object') return null;
                out.push([k, v === null ? '' : String(v)]);
            }
            return out;
        }

        // Current variables as pairs, or null when the JSON editor holds something invalid
        function varsPairs() {
            return jsonMode ? parseJsonPairs() : kvPairs();
        }

        // Write the JSON object of strings (or "") into the hidden field; false when invalid
        function serializeVars() {
            var hidden = $('variables');
            if (!hidden) return true;
            var pairs = varsPairs();
            if (pairs === null) return false;
            hidden.value = pairs.length ? JSON.stringify(pairsToObject(pairs)) : '';
            return true;
        }

        function setJsonMode(on) {
            jsonMode = on;
            if (!jsonEl) return;
            jsonEl.hidden = !on;
            if (kv) kv.hidden = on;
            var add = root.querySelector('[data-act="kv-add"]');
            if (add) add.hidden = on;
            var tog = root.querySelector('[data-act="kv-json"]');
            if (tog) tog.textContent = on ? t('cf.kv_list') : t('cf.kv_json');
        }

        function toggleJson() {
            if (!jsonEl) return;
            if (!jsonMode) {
                jsonEl.value = JSON.stringify(pairsToObject(kvPairs()), null, 2);
                setErr('variables', '');
                setJsonMode(true);
                jsonEl.focus();
            } else {
                var pairs = parseJsonPairs();
                if (pairs === null) { setErr('variables', 'cmp.wz_err_variables'); return; } // stay in JSON mode
                setKv(pairs);
                setJsonMode(false);
            }
        }

        // ---------- recipient lists (row "to") ----------

        var listsBox = $('cf-lists');

        function selectedList() {
            if (!listsBox) return null;
            var r = listsBox.querySelector('input[type="radio"]:checked');
            if (!r) return null;
            var card = r.closest('.cf-list');
            return {
                id: r.value,
                name: card.getAttribute('data-name') || '',
                active: parseInt(card.getAttribute('data-active'), 10) || 0,
                total: parseInt(card.getAttribute('data-total'), 10) || 0
            };
        }

        function renderLists() {
            if (!listsBox) return;
            all(listsBox, '.cf-list').forEach(function (card) {
                var active = parseInt(card.getAttribute('data-active'), 10) || 0;
                var total = parseInt(card.getAttribute('data-total'), 10) || 0;
                var small = card.querySelector('[data-role="inactive"]');
                var inactive = Math.max(total - active, 0);
                if (small) small.textContent = t('cmp.wz_will_get') + (inactive ? ' · ' + t('cmp.wz_inactive', { n: inactive }) : '');
                var inp = card.querySelector('input');
                card.classList.toggle('sel', !!inp && inp.checked);
            });
        }

        // ---------- when: schedule, pilot, time zone (row "when") ----------

        var whenRadios = all(root, 'input[name="cf_when"]');
        var schedEl = $('scheduled_at');
        var pilotEl = $('dry_run');
        var limitEl = $('dry_run_limit');
        var tzLabel = '';

        function isLater() { return whenRadios.some(function (r) { return r.checked && r.value === 'later'; }); }

        function renderWhen() {
            if (!schedEl) return;
            var later = isLater();
            var box = $('cf-when-later');
            if (box) box.hidden = !later;
            schedEl.disabled = !later; // a disabled field is not submitted, so the server sees "now"
            if (!later) setErr('scheduled_at', '');
        }

        function renderPilot() {
            if (!pilotEl) return;
            var box = $('cf-pilot');
            if (box) box.hidden = !pilotEl.checked;
            limitEl.disabled = !pilotEl.checked;
            if (!pilotEl.checked) setErr('dry_run_limit', '');
        }

        // The schedule is entered in the browser's time zone; the server needs it to convert to UTC
        (function () {
            var tzEl = $('tz');
            if (!tzEl) return;
            var tz = '';
            try { tz = Intl.DateTimeFormat().resolvedOptions().timeZone || ''; } catch (e) { /* keep empty */ }
            tzEl.value = tz;
            var offset = -new Date().getTimezoneOffset();
            var sign = offset >= 0 ? '+' : '-';
            var hh = String(Math.floor(Math.abs(offset) / 60)).padStart(2, '0');
            var mm = String(Math.abs(offset) % 60).padStart(2, '0');
            tzLabel = (tz ? tz + ' ' : '') + '(UTC' + sign + hh + ':' + mm + ')';
            var lbl = $('cf-tz-label');
            if (lbl) lbl.textContent = tzLabel;
        })();

        function pilotCount() {
            if (!pilotEl || !pilotEl.checked) return null;
            var n = parseInt(limitEl.value, 10);
            return n >= 1 ? Math.min(n, 100) : null;
        }

        function recipientCount() {
            var list = selectedList();
            if (!list) return 0;
            var pilot = pilotCount();
            return pilot === null ? list.active : Math.min(pilot, list.active);
        }

        // Russian wants "1 получателю" but "5 получателям"; English "1 recipient" / "5 recipients"
        function isOne(n) {
            if (window.I18n && I18n.getLang() === 'ru') return n % 10 === 1 && n % 100 !== 11;
            return n === 1;
        }

        // ---------- summaries ----------

        function out(row) { return row.querySelector('[data-out]'); }

        function mini(id) {
            var s = el('span', 'cf-mini');
            if (id) {
                var f = document.createElement('iframe');
                f.src = '/templates/' + encodeURIComponent(id) + '/frame';
                f.setAttribute('sandbox', '');
                f.setAttribute('loading', 'lazy');
                f.setAttribute('tabindex', '-1');
                f.setAttribute('aria-hidden', 'true');
                f.title = '';
                s.appendChild(f);
            }
            return s;
        }

        function verBlock(v, label, pct) {
            var vi = verInputs(v);
            var info = tplInfo(vi.tpl.value);
            var box = el('div', 'cf-ver');
            box.appendChild(mini(vi.tpl.value));
            var text = el('div');
            if (label) text.appendChild(el('b', '', label));
            text.appendChild(document.createTextNode(info ? info.name : '—'));
            var subj = vi.sub.value.trim() || (info ? info.subject : '');
            if (subj) {
                text.appendChild(document.createElement('br'));
                text.appendChild(el('span', 'cf-sub', '«' + subj + '»'));
            }
            if (pct !== undefined) {
                text.appendChild(document.createElement('br'));
                text.appendChild(el('span', 'cf-pct', t('cf.pct_recipients', { n: pct })));
            }
            box.appendChild(text);
            return box;
        }

        function sub(text) { return el('span', 'cf-sub', text); }

        var summary = {
            from: function (o) {
                var name = trimmed('from_name');
                var email = trimmed('from_email');
                o.appendChild(document.createTextNode(name ? name + ' <' + email + '>' : email));
                o.appendChild(document.createElement('br'));
                o.appendChild(sub('Reply-To: ' + (trimmed('reply_to') || email)));
            },
            content: function (o) {
                if (!tplA || !tplA.value) { o.appendChild(sub(t('cf.no_template'))); return; }
                if (abOn()) {
                    var a = parseInt(splitEl.value, 10) || 50;
                    var box = el('div', 'cf-vers');
                    box.appendChild(verBlock('a', t('cf.version', { v: 'A' }), a));
                    box.appendChild(verBlock('b', t('cf.version', { v: 'B' }), 100 - a));
                    o.appendChild(box);
                } else {
                    o.appendChild(verBlock('a'));
                }
            },
            vars: function (o) {
                var pairs = varsPairs() || [];
                var box = el('span', 'cf-kvs');
                if (!pairs.length) { box.appendChild(sub(t('cf.none'))); o.appendChild(box); return; }
                pairs.forEach(function (p) { box.appendChild(el('code', '', p[0] + ' = ' + p[1])); });
                o.appendChild(box);
            },
            track: function (o) {
                var op = !!$('track_opens') && $('track_opens').checked;
                var cl = !!$('track_clicks') && $('track_clicks').checked;
                var utm = !!$('add_utm') && $('add_utm').checked;
                var what = op && cl ? 'cf.track_both' : op ? 'cf.track_opens' : cl ? 'cf.track_clicks' : 'cf.track_none';
                o.textContent = t(what) + t(utm ? 'cf.utm_on' : 'cf.utm_off');
            },
            about: function (o) {
                o.appendChild(document.createTextNode(campaignName()));
                var d = trimmed('description');
                if (d) {
                    o.appendChild(document.createElement('br'));
                    o.appendChild(sub(d));
                }
            },
            to: function (o) {
                var list = selectedList();
                if (!list) return;
                o.appendChild(document.createTextNode(list.name));
                o.appendChild(document.createElement('br'));
                o.appendChild(sub(t('cf.will_get', { n: list.active })));
            },
            check: function (o) {
                o.textContent = t('cf.reviewed') + (testSentTo ? t('cf.test_sent', { to: testSentTo }) : '');
            },
            when: function (o) {
                var text = t('cmp.wz_sum_now');
                if (isLater() && schedEl && schedEl.value) {
                    var lang = window.I18n ? I18n.getLang() : 'en';
                    var d = new Date(schedEl.value);
                    text = d.toLocaleString(lang === 'ru' ? 'ru-RU' : undefined, { dateStyle: 'long', timeStyle: 'short' }) + ' (' + tzLabel + ')';
                }
                o.appendChild(document.createTextNode(text));
                var pilot = pilotCount();
                if (pilot !== null) {
                    o.appendChild(document.createElement('br'));
                    o.appendChild(sub(t('cf.pilot_first', { n: pilot })));
                }
            }
        };

        function fill(row) {
            var o = out(row);
            var f = summary[rowName(row)];
            if (!o || !f) return;
            o.textContent = '';
            f(o);
            built[rowName(row)] = true;
        }

        function refillBuilt() {
            rows.forEach(function (r) { if (built[rowName(r)]) fill(r); });
        }

        // ---------- campaign name and page title ----------

        var titleEl = $('cf-page-title');
        var savedName = trimmed('name'); // for document.title in edit mode

        function pad2(n) { return n < 10 ? '0' + n : String(n); }

        // Same rule as the server: subject of version A without {{variables}} + ", DD.MM.YYYY"
        function autoName() {
            var info = tplInfo(tplA ? tplA.value : '');
            var subject = trimmed('subject') || (info ? info.subject : '');
            if (!subject && !info) return '';
            var s = subject.replace(/\{\{[^}]*\}\}/g, '').replace(/^[\s,.!:;-]+|[\s,.!:;-]+$/g, '').replace(/\s{2,}/g, ' ');
            var d = new Date();
            var date = pad2(d.getDate()) + '.' + pad2(d.getMonth() + 1) + '.' + d.getFullYear();
            return s ? s + ', ' + date : 'Campaign ' + date;
        }

        function campaignName() {
            return trimmed('name') || (mode === 'create' ? autoName() : '');
        }

        // In create the name field stays empty; the title shows what the server will compute
        function renderTitle() {
            if (mode !== 'create') return;
            if (titleEl) titleEl.textContent = campaignName() || t('cmp.new_campaign_title');
            if (rowMap.about && built.about) fill(rowMap.about);
        }
        var nameEl = $('name');
        if (nameEl && mode === 'create') nameEl.addEventListener('input', renderTitle);
        if (subA && mode === 'create') subA.addEventListener('input', renderTitle);

        // After "about" is saved in edit/send mode
        function updatePageHeader() {
            var name = trimmed('name');
            var desc = trimmed('description');
            if (titleEl) titleEl.textContent = name;
            var d = $('cf-page-desc');
            if (d) { d.textContent = desc; d.hidden = !desc; }
            if (savedName && document.title.indexOf(savedName) !== -1) document.title = document.title.replace(savedName, name);
            savedName = name;
        }

        // ---------- row controller ----------

        function hasTemplate() { return !tplA || !!tplA.value; }

        function contentDone() { return mode === 'create' ? cur > 1 : true; }

        function primaryLabel(row) {
            if (isEditRow(row)) return t('cf.save');
            var i = steps.indexOf(row);
            if (i === -1) return t('cf.done'); // optional rows in create
            return i < cur || i === steps.length - 1 ? t('cf.done') : t('cf.next');
        }

        // Dots, state classes, buttons of every row
        function renderRows() {
            rows.forEach(function (r) {
                var name = rowName(r);
                var i = steps.indexOf(r);
                var dot = r.querySelector('.cf-dot');
                var isOpen = r === open;
                var locked = false;
                var done = false;
                var state = 'ok';
                var text = '✓';
                if (i !== -1) {
                    done = i < cur;
                    locked = i > cur;
                    state = done ? 'ok' : i === cur ? 'cur' : 'todo';
                    text = done ? '✓' : String(i + 1);
                } else if (mode === 'create' && OPTIONAL.indexOf(name) !== -1) {
                    locked = !contentDone();
                    done = !locked;
                    state = 'opt';
                    text = '';
                } else if (isEditRow(r)) {
                    done = true;
                    // Optional rows always have a value; a check there says nothing
                    if (OPTIONAL.indexOf(name) !== -1) { state = 'opt'; text = ''; }
                    if (name === 'content' && !hasTemplate()) { state = 'warn'; text = '!'; }
                }
                if (dot) { dot.className = 'cf-dot ' + state; dot.textContent = text; }
                r.classList.toggle('is-open', isOpen);
                r.classList.toggle('is-locked', locked);
                r.classList.toggle('is-done', done && !isOpen);
                var btn = r.querySelector('[data-act="open"]');
                if (btn) {
                    btn.hidden = isOpen || locked;
                    btn.setAttribute('aria-expanded', isOpen ? 'true' : 'false');
                }
                var body = r.querySelector('.cf-body');
                if (body) body.hidden = !isOpen;
                var cancel = r.querySelector('[data-act="cancel"]');
                if (cancel) { cancel.hidden = !isEditRow(r); cancel.textContent = t('cf.cancel'); }
                var primary = r.querySelector('[data-act="primary"]');
                if (primary) primary.textContent = primaryLabel(r);
            });
            renderFinal();
        }

        // Fields of a row that take part in its snapshot (kv editor and gallery search are handled apart)
        function snapFields(row) {
            return all(row, 'input, textarea, select').filter(function (f) {
                return !f.closest('.cf-kv-row') && f !== jsonEl && f !== searchEl;
            });
        }

        // State of a row's fields, so "Cancel" can put everything back
        function capture(row) {
            return {
                fields: snapFields(row).map(function (f) {
                    return { el: f, value: f.value, checked: f.checked, disabled: f.disabled, touched: f.dataset.touched };
                }),
                kv: rowName(row) === 'vars' ? (varsPairs() || kvPairs()) : null,
                json: jsonMode,
                ver: curVer
            };
        }

        function restore(row, snap) {
            snap.fields.forEach(function (s) {
                s.el.value = s.value;
                s.el.checked = s.checked;
                s.el.disabled = s.disabled;
                s.el.dataset.touched = s.touched || '';
                s.el.removeAttribute('aria-invalid');
            });
            var name = rowName(row);
            if (name === 'vars') { setKv(snap.kv); setJsonMode(false); }
            if (name === 'content') { curVer = snap.ver; renderAB(); }
            if (name === 'to') renderLists();
            if (name === 'when') { renderWhen(); renderPilot(); }
            clearRowErrors(row);
        }

        // Body of the row as a form-urlencoded string: named, enabled inputs only
        function serializeRow(row) {
            if (rowName(row) === 'vars') serializeVars();
            var p = new URLSearchParams();
            all(row, 'input[name], textarea[name], select[name]').forEach(function (f) {
                if (f.disabled) return;
                if ((f.type === 'checkbox' || f.type === 'radio') && !f.checked) return;
                p.append(f.name, f.value);
            });
            return p;
        }

        function focusFirstField(row) {
            var f = row.querySelector('.cf-fields input[type="radio"]:checked');
            if (!f) {
                f = all(row, '.cf-fields input, .cf-fields textarea, .cf-fields select, .cf-fields button').filter(function (x) {
                    return x.type !== 'hidden' && !x.disabled && x.offsetParent !== null;
                })[0];
            }
            if (f && f.focus) f.focus({ preventScroll: false });
        }

        function openRow(row, focus) {
            if (open && open !== row) closeRow(open, false, true);
            open = row;
            clearRowErrors(row);
            if (isEditRow(row)) {
                row._cfSnap = capture(row);
                row._cfBase = serializeRow(row).toString();
            }
            renderRows();
            var name = rowName(row);
            if (name === 'content') { showVer(curVer); updateScale(); }
            if (name === 'check') enterReview();
            if (focus !== false) focusFirstField(row);
        }

        // Close a row; with revert the fields go back to what they were on open
        function closeRow(row, focusBtn, revert) {
            if (revert && isEditRow(row) && row._cfSnap) restore(row, row._cfSnap);
            if (open === row) open = null;
            renderRows();
            if (focusBtn) {
                var b = row.querySelector('[data-act="open"]');
                if (b && !b.hidden) b.focus();
            }
        }

        // After a row is done: the current step opens again (steps exist in create and send)
        function backToCurrent(focus) {
            if (steps.length && cur < steps.length) openRow(steps[cur], focus);
            else renderRows();
        }

        function flash(row) {
            var name = row.querySelector('.cf-name');
            if (!name) return;
            var s = el('span', 'cf-saved', ' ' + t('cf.saved'));
            s.setAttribute('role', 'status');
            name.appendChild(s);
            setTimeout(function () { if (s.parentNode) s.parentNode.removeChild(s); }, 2000);
        }

        function onPrimary(row) {
            if (isEditRow(row)) { saveRow(row); return; }
            if (!validateRow(row, 'send')) { focusFirstError(row); return; }
            var i = steps.indexOf(row);
            fill(row);
            if (i !== -1 && i === cur) cur++;
            if (mode === 'create' && rowMap.about) fill(rowMap.about);
            closeRow(row, false, false);
            backToCurrent(true);
            if (!open) { var b = row.querySelector('[data-act="open"]'); if (b) b.focus(); }
        }

        function onCancel(row) {
            clearRowErrors(row);
            closeRow(row, true, true);
            if (mode === 'send') backToCurrent(false);
        }

        // Edit rows: POST the row's fields, JSON answer
        function saveRow(row) {
            if (!validateRow(row, 'save')) { focusFirstError(row); return; }
            var primary = row.querySelector('[data-act="primary"]');
            var name = rowName(row);
            var body = serializeRow(row);
            showRowError(row, '');
            if (primary) primary.disabled = true;
            fetch('/campaigns/' + encodeURIComponent(campaignId) + '/section/' + name, {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                credentials: 'same-origin',
                body: body.toString()
            }).then(function (r) {
                return r.json().catch(function () { return {}; }).then(function (data) { return { ok: r.ok, data: data }; });
            }).then(function (res) {
                if (res.ok && res.data && res.data.ok) {
                    row._cfSnap = null;
                    // the list is what the row reopens with: bring it in line with the saved JSON
                    if (name === 'vars' && jsonMode) { setKv(parseJsonPairs() || []); setJsonMode(false); }
                    fill(row);
                    if (name === 'about') updatePageHeader();
                    closeRow(row, true, false);
                    flash(row);
                    if (mode === 'send') backToCurrent(false);
                } else {
                    showRowError(row, (res.data && res.data.error) || t('cf.save_failed'));
                }
            }).catch(function () {
                showRowError(row, t('cf.save_failed'));
            }).then(function () { if (primary) primary.disabled = false; });
        }

        // ---------- validation ----------

        function validateRow(row, action) {
            var ok = true;
            function fail(name, key) { setErr(name, key); ok = false; }
            clearRowErrors(row);
            switch (rowName(row)) {
            case 'from':
                var from = trimmed('from_email');
                if (!from) fail('from_email', 'cmp.wz_err_from');
                else if (!EMAIL_RE.test(from)) fail('from_email', 'cmp.wz_err_email');
                var replyTo = trimmed('reply_to');
                if (replyTo && !EMAIL_RE.test(replyTo)) fail('reply_to', 'cmp.wz_err_email');
                break;
            case 'content':
                if (!tplA.value) { fail('template_id', 'cmp.wz_err_template'); showVer('a'); }
                else if (abOn() && !tplB.value) { fail('template_id_b', 'cmp.wz_err_template_b'); showVer('b'); }
                break;
            case 'vars':
                if (!serializeVars()) fail('variables', 'cmp.wz_err_variables');
                break;
            case 'about':
                if (mode !== 'create' && !trimmed('name')) fail('name', 'cmp.wz_err_name');
                break;
            case 'to':
                if (!selectedList()) fail('recipient_list_id', 'cmp.wz_err_list');
                break;
            case 'when':
                if (isLater()) {
                    var v = schedEl.value;
                    if (!v) fail('scheduled_at', 'cmp.wz_err_when');
                    else if (new Date(v).getTime() <= Date.now()) fail('scheduled_at', 'cmp.wz_err_past');
                }
                if (pilotEl.checked) {
                    var n = parseInt(limitEl.value, 10);
                    if (!(n >= 1 && n <= 100)) fail('dry_run_limit', 'cmp.wz_err_pilot');
                }
                if (action === 'send' && !root.querySelector('input[name="servers"]:checked')) {
                    fail('servers', 'cmp.wz_err_servers');
                    var more = row.querySelector('details.cf-more');
                    if (more) more.open = true;
                }
                break;
            }
            return ok;
        }

        // ---------- check row: preview, test email, readiness ----------

        var pvVer = 'a';
        var devWidth = 'pc';
        var previewCache = {};
        var previewSeq = 0;
        var frame = $('cf-preview');

        function variantInfo(v) {
            var vi = verInputs(v === 'b' && abOn() ? 'b' : 'a');
            var info = tplInfo(vi.tpl.value) || { id: vi.tpl.value, name: '', subject: '', unsub: false };
            return { id: vi.tpl.value, name: info.name, subject: vi.sub.value.trim() || info.subject, unsub: info.unsub };
        }

        function showPreviewMsg(text, isError) {
            var msg = $('cf-preview-msg');
            if (msg) {
                msg.textContent = text || '';
                msg.hidden = !text;
                msg.classList.toggle('err', !!isError);
            }
            if (frame) frame.hidden = !!text;
        }

        function loadPreview() {
            if (!frame) return;
            var info = variantInfo(pvVer);
            var subj = $('cf-pv-subject');
            if (subj) subj.textContent = info.subject;
            serializeVars();
            var vars = val('variables').trim();
            var key = info.id + '|' + vars;
            if (previewCache[key] !== undefined) {
                showPreviewMsg('');
                frame.srcdoc = previewCache[key];
                return;
            }
            var seq = ++previewSeq;
            showPreviewMsg(t('cmp.wz_loading'), false);
            fetch('/campaigns/wizard/preview', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({ template_id: info.id, variables: vars })
            }).then(function (r) {
                if (!r.ok) return r.text().then(function (txt) { throw new Error(txt); });
                return r.text();
            }).then(function (html) {
                if (window.SendryPreview) html = SendryPreview.stripWrapperDarkModeCSS(html);
                previewCache[key] = html;
                if (seq !== previewSeq) return;
                showPreviewMsg('');
                frame.srcdoc = html;
            }).catch(function (err) {
                if (seq !== previewSeq) return;
                showPreviewMsg((err && err.message) || t('cmp.wz_preview_failed'), true);
            });
        }

        function renderDevice() {
            var stage = $('cf-stage');
            if (stage) stage.classList.toggle('is-phone', devWidth === 'phone');
            var dev = $('cf-dev');
            if (dev) all(dev, 'button').forEach(function (b) {
                var on = b.getAttribute('data-dev') === devWidth;
                b.classList.toggle('on', on);
                b.setAttribute('aria-pressed', on ? 'true' : 'false');
            });
        }

        function renderPvVer() {
            var box = $('cf-pv-ver');
            if (!box) return;
            box.hidden = !abOn();
            if (!abOn()) pvVer = 'a';
            all(box, 'button').forEach(function (b) {
                var on = b.getAttribute('data-ver') === pvVer;
                b.classList.toggle('on', on);
                b.setAttribute('aria-pressed', on ? 'true' : 'false');
            });
        }

        var devBox = $('cf-dev');
        if (devBox) devBox.addEventListener('click', function (e) {
            var b = e.target.closest('button');
            if (!b) return;
            devWidth = b.getAttribute('data-dev');
            renderDevice();
        });
        var pvBox = $('cf-pv-ver');
        if (pvBox) pvBox.addEventListener('click', function (e) {
            var b = e.target.closest('button');
            if (!b) return;
            pvVer = b.getAttribute('data-ver');
            renderPvVer();
            loadPreview();
        });

        // Readiness list: display only, it never blocks sending
        function readyRow(state, title, desc, link) {
            var row = el('div', 'cf-ready-row');
            var ico = el('span', 'cf-ico ' + state, state === 'ok' ? '✓' : '!');
            ico.setAttribute('aria-hidden', 'true');
            var body = el('div');
            body.appendChild(el('div', 'cf-ready-t', title));
            if (desc || link) {
                var d = el('div', 'cf-ready-d', desc || '');
                if (link) {
                    if (desc) d.appendChild(document.createTextNode(' '));
                    if (link.href) {
                        var a = el('a', '', link.text);
                        a.href = link.href;
                        d.appendChild(a);
                    } else {
                        d.appendChild(document.createTextNode(link.text));
                    }
                }
                body.appendChild(d);
            }
            row.appendChild(ico);
            row.appendChild(body);
            row.appendChild(el('span', 'sr-only', state === 'ok' ? t('cmp.wz_rdy_ok') : t('cmp.wz_rdy_warn')));
            return row;
        }

        function adminLink(href, key) {
            return cfg.isAdmin ? { href: href, text: t(key) } : { text: t('cmp.wz_ask_admin') };
        }

        function renderReadiness() {
            var box = $('cf-ready');
            if (!box) return;
            box.textContent = '';

            // Unsubscribe link in every chosen version
            var versions = [variantInfo('a')];
            if (abOn()) versions.push(variantInfo('b'));
            var allUnsub = versions.every(function (v) { return v.unsub; });
            if (allUnsub) box.appendChild(readyRow('ok', t('cmp.wz_rdy_unsub_ok')));
            else box.appendChild(readyRow('warn', t('cmp.wz_rdy_unsub_no'), t('cmp.wz_rdy_unsub_no_d')));

            // Proxying domain
            if (!cfg.noPublicURL) box.appendChild(readyRow('ok', t('cmp.wz_rdy_proxy_ok')));
            else box.appendChild(readyRow('warn', t('cmp.wz_rdy_proxy_no'), t('cmp.wz_rdy_proxy_no_d'), adminLink('/settings/tracking', 'trk.settings_link')));

            // Sender domain
            var from = trimmed('from_email').toLowerCase();
            var at = from.lastIndexOf('@');
            if (at > 0) {
                var domain = from.slice(at + 1);
                var d = senderDomains[domain];
                var params = { domain: domain };
                if (!d) {
                    box.appendChild(readyRow('warn', t('cmp.wz_rdy_dom_unknown', params), t('cmp.wz_rdy_dom_unknown_d'), adminLink('/domains', 'cmp.wz_open_domains')));
                } else {
                    var deployed = d.Deployed !== undefined ? d.Deployed : d.deployed;
                    var dkim = d.DKIMEnabled !== undefined ? d.DKIMEnabled : d.dkim_enabled;
                    if (!deployed) box.appendChild(readyRow('warn', t('cmp.wz_rdy_dom_nodeploy', params), t('cmp.wz_rdy_dom_nodeploy_d'), adminLink('/domains', 'cmp.wz_open_domains')));
                    else if (!dkim) box.appendChild(readyRow('warn', t('cmp.wz_rdy_dom_nodkim', params), t('cmp.wz_rdy_dom_nodkim_d'), adminLink('/dkim', 'cmp.wz_open_dkim')));
                    else box.appendChild(readyRow('ok', t('cmp.wz_rdy_dom_ok', params), t('cmp.wz_rdy_dom_ok_d')));
                }
            }
        }

        // The check row has just been opened
        function enterReview() {
            renderPvVer();
            renderDevice();
            loadPreview();
            renderReadiness();
        }

        // Test email: sends the version currently previewed
        var testBtn = $('cf-test-btn');
        if (testBtn) testBtn.addEventListener('click', function () {
            var res = $('cf-test-result');
            var to = trimmed('cf-test-to');
            function show(text, ok) {
                res.textContent = text;
                res.hidden = false;
                res.classList.toggle('ok', ok === true);
                res.classList.toggle('err', ok === false);
            }
            if (!EMAIL_RE.test(to)) { show(t('cmp.wz_err_email'), false); return; }
            var info = variantInfo(pvVer);
            serializeVars();
            // the first server picked for the run, if the page has the choice
            var srv = root.querySelector('input[name="servers"]:checked');
            testBtn.disabled = true;
            show(t('cmp.wz_test_sending'));
            fetch('/campaigns/wizard/test', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'same-origin',
                body: JSON.stringify({
                    template_id: info.id,
                    subject: info.subject,
                    from_email: trimmed('from_email'),
                    from_name: trimmed('from_name'),
                    reply_to: trimmed('reply_to'),
                    variables: val('variables').trim(),
                    server: srv ? srv.value : '',
                    to: to
                })
            }).then(function (r) {
                return r.json().catch(function () { return {}; }).then(function (data) { return { ok: r.ok, data: data }; });
            }).then(function (r2) {
                if (r2.ok && r2.data.ok) { testSentTo = to; show(t('cmp.wz_test_ok', { to: to }), true); }
                else show(r2.data.error || t('cmp.wz_test_failed'), false);
            }).catch(function () {
                show(t('cmp.wz_test_failed'), false);
            }).then(function () { testBtn.disabled = false; });
        });

        // ---------- final buttons ----------

        // Letter row left open with unsaved changes (send mode); null when none
        function dirtyLetterRow() {
            if (mode !== 'send' || !open || !isEditRow(open)) return null;
            return serializeRow(open).toString() !== open._cfBase ? open : null;
        }

        function renderFinal() {
            var sendBtn = $('cf-send');
            var saveBtn = $('cf-save');
            var help = $('cf-final-help');
            if (!sendBtn) return;
            var allDone = cur >= steps.length;
            var helpText = '';
            if (mode === 'create') {
                if (saveBtn) saveBtn.disabled = !contentDone();
                sendBtn.disabled = !allDone;
                helpText = allDone ? '' : contentDone() ? t('cf.help_can_save') : t('cf.help_fill_first');
            } else {
                var tpl = hasTemplate();
                sendBtn.disabled = !allDone || !tpl;
                helpText = !tpl ? t('cf.help_no_template') : allDone ? '' : t('cf.help_finish_steps');
            }
            if (allDone) {
                var n = recipientCount();
                sendBtn.textContent = t(isOne(n) ? 'cmp.wz_send_one' : 'cmp.wz_send_many', { n: n });
            } else {
                sendBtn.textContent = t('cf.send');
            }
            if (help) { help.textContent = helpText; help.hidden = !helpText; }
        }

        // ---------- events ----------

        root.addEventListener('click', function (e) {
            var btn = e.target.closest('[data-act]');
            if (!btn || !root.contains(btn)) return;
            var act = btn.getAttribute('data-act');
            var row = btn.closest('.cf-row');
            if (act === 'open' && row) {
                if (!row.classList.contains('is-locked')) openRow(row, true);
            } else if (act === 'cancel' && row) {
                onCancel(row);
            } else if (act === 'primary' && row) {
                onPrimary(row);
            } else if (act === 'ab') {
                setAB(!abOn());
                if (abOn() && gal) { var card = gal.querySelector('.cf-tpl:not([hidden])'); if (card) card.focus(); }
            } else if (act === 'kv-add') {
                var r = kvRow('', '');
                kv.appendChild(r);
                r.querySelector('input').focus();
            } else if (act === 'kv-del') {
                var kr = btn.closest('.cf-kv-row');
                if (kr) kr.parentNode.removeChild(kr);
                if (!kv.querySelector('.cf-kv-row')) kv.appendChild(kvRow('', ''));
            } else if (act === 'kv-json') {
                toggleJson();
            } else if (act === 'ver-del') {
                deleteVersion(btn.getAttribute('data-del'));
            }
        });

        // Version tabs of the content row
        all(root, '.cf-vtabs button[data-ver]').forEach(function (b) {
            b.addEventListener('click', function () { showVer(b.getAttribute('data-ver')); });
        });

        // Enter inside a field must not submit the whole form; inside an open row it means "Next" / "Save"
        root.addEventListener('keydown', function (e) {
            var tgt = e.target;
            if (e.key === 'Escape' && open && isEditRow(open) && open.contains(tgt) && tgt.type !== 'search') {
                onCancel(open);
                return;
            }
            if (e.key !== 'Enter' || !tgt || (tgt.tagName !== 'INPUT' && tgt.tagName !== 'SELECT')) return;
            e.preventDefault();
            if (tgt.type === 'search' || tgt.closest('.cf-kv-row') || tgt.type === 'radio') return;
            var row = tgt.closest('.cf-row');
            if (row && row === open) onPrimary(row);
        });

        root.addEventListener('change', function (e) {
            if (e.target.closest('#cf-lists')) renderLists();
            if (e.target.name === 'cf_when') { renderWhen(); }
            if (e.target.id === 'dry_run') renderPilot();
            renderFinal();
        });
        root.addEventListener('input', function (e) {
            if (e.target.id === 'dry_run_limit' || e.target.id === 'scheduled_at') renderFinal();
        });

        // ---------- submit (create and send) ----------

        if (form) form.addEventListener('submit', function (e) {
            var action = e.submitter && e.submitter.value ? e.submitter.value : 'send';
            if (submitting) { e.preventDefault(); return; }

            // Send mode: an unsaved letter row would be sent as it was before the edit
            var dirty = dirtyLetterRow();
            if (dirty) {
                e.preventDefault();
                var nameEl2 = dirty.querySelector('.cf-name');
                var help = $('cf-final-help');
                if (help) {
                    help.textContent = t('cf.help_unsaved', { row: nameEl2 ? nameEl2.textContent.replace(/\s*$/, '') : rowName(dirty) });
                    help.hidden = false;
                }
                return;
            }

            // Re-check every row: the user may have gone back and broken something
            var need = mode === 'create'
                ? (action === 'save' ? ['from', 'content', 'vars'] : ['from', 'content', 'vars', 'to', 'when'])
                : ['to', 'when'];
            for (var i = 0; i < need.length; i++) {
                var row = rowMap[need[i]];
                if (!row) continue;
                if (!validateRow(row, action)) {
                    e.preventDefault();
                    openRow(row, false);
                    focusFirstError(row);
                    return;
                }
            }
            if (mode === 'create' && !hasTemplate()) { e.preventDefault(); return; }

            serializeVars();
            // The offset at the scheduled date, in case daylight saving time changes before it
            var offEl = $('tz_offset');
            if (offEl) {
                var v = isLater() && schedEl ? schedEl.value : '';
                offEl.value = String((v ? new Date(v) : new Date()).getTimezoneOffset());
            }
            // Disable the buttons after the browser has collected the form data (a disabled submitter is not sent)
            submitting = true;
            setTimeout(function () {
                all(form, 'button[type="submit"]').forEach(function (b) { b.disabled = true; });
                if (document.getElementById('cf-send') && !form.contains($('cf-send'))) $('cf-send').disabled = true;
                if ($('cf-save')) $('cf-save').disabled = true;
            }, 0);
        });

        // Back from the result page restores this one from the bfcache with the
        // buttons still locked: unlock them, renderFinal sets what is allowed
        window.addEventListener('pageshow', function (e) {
            if (!e.persisted || !submitting) return;
            submitting = false;
            all(form, 'button[type="submit"]').forEach(function (b) { b.disabled = false; });
            if ($('cf-send')) $('cf-send').disabled = false;
            if ($('cf-save')) $('cf-save').disabled = false;
            renderFinal();
        });

        // Language switch: refresh the texts built by script
        all(document, '.lang-btn').forEach(function (b) {
            b.addEventListener('click', function () {
                setTimeout(function () {
                    renderLists();
                    refillBuilt();
                    renderAB();
                    renderTitle();
                    setJsonMode(jsonMode);
                    renderRows();
                    if (open && rowName(open) === 'check') renderReadiness();
                }, 0);
            });
        });

        // ---------- initial state ----------

        renderLists();
        renderWhen();
        renderPilot();
        renderTitle();
        if (kv && !kv.querySelector('.cf-kv-row')) setKv([]);
        // Summaries: every row in edit/send, optional rows in create (they are shown filled with defaults)
        rows.forEach(function (r) {
            var name = rowName(r);
            var isOptCreate = mode === 'create' && OPTIONAL.indexOf(name) !== -1;
            if (isEditRow(r) || isOptCreate) fill(r);
        });
        if (mode === 'create' && rowMap.about) fill(rowMap.about);
        if (steps.length) openRow(steps[0], false);
        else renderRows();
        updateScale();
        // The thumbnail width is known only after layout and i18n have run
        setTimeout(updateScale, 0);
    }

    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
    else init();
})();
