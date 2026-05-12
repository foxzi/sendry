(function() {
    'use strict';

    var data = window.__builderData;
    if (!data) return;

    var categories = data.categories || [];
    var wrapper = data.wrapper || '';

    var blockMap = {};
    categories.forEach(function(cat) {
        (cat.blocks || []).forEach(function(block) {
            blockMap[block.id] = block;
        });
    });

    var canvasBlocks = [];
    var uidCounter = 0;

    var initialRefs = data.initialBlocks || (data.initialBlockIDs || []).map(function(id) {
        return {block_id: id, gap_height: 0, gap_color: ''};
    });
    if (initialRefs.length) {
        initialRefs.forEach(function(ref) {
            var block = blockMap[ref.block_id];
            if (!block) return; // block deleted or unavailable — silently skip
            canvasBlocks.push({
                uid: ++uidCounter,
                blockId: block.id,
                name: block.name,
                html: block.html,
                borderRadius: block.border_radius || 0,
                paddingV: block.padding_v || 0,
                paddingH: block.padding_h || 0,
                background: block.background || '',
                gapHeight: ref.gap_height || 0,
                gapColor: ref.gap_color || '',
                condition: ref.condition || ''
            });
        });

        Promise.resolve().then(function() { renderCanvas(); });
    }

    var canvasContainer = document.getElementById('canvas-blocks');
    var canvasEmpty = document.getElementById('canvas-empty');
    var blockCount = document.getElementById('block-count');
    var htmlHidden = document.getElementById('html-hidden');
    var variablesHidden = document.getElementById('variables-hidden');

    var blockRefsHidden = document.getElementById('block-refs-hidden');
    var form = document.getElementById('builder-form');

    var canvasBuild = document.getElementById('canvas-build');
    var canvasPreview = document.getElementById('canvas-preview');
    var canvasCode = document.getElementById('canvas-code');
    var previewFrame = document.getElementById('preview-frame');
    var codeOutput = document.getElementById('code-output');

    document.querySelectorAll('.canvas-tab').forEach(function(tab) {
        tab.addEventListener('click', function() {
            document.querySelectorAll('.canvas-tab').forEach(function(t) { t.classList.remove('active'); });
            this.classList.add('active');

            var target = this.getAttribute('data-tab');
            canvasBuild.style.display = target === 'build' ? '' : 'none';
            canvasPreview.style.display = target === 'preview' ? '' : 'none';
            canvasCode.style.display = target === 'code' ? '' : 'none';

            if (target === 'preview') {
                var dataTa = document.getElementById('builder-preview-data');
                if (dataTa && !dataTa.value.trim()) {
                    try { dataTa.value = JSON.stringify(buildSkeleton(assembleHTML()), null, 2); } catch (e) {}
                }
                updatePreview();
            }
            if (target === 'code') updateCode();
        });
    });

    document.querySelectorAll('.block-card').forEach(function(card) {
        card.addEventListener('click', function() {
            var blockId = this.getAttribute('data-block-id');
            var block = blockMap[blockId];
            if (!block) return;
            addBlock(block);
        });
    });

    function addBlock(block) {
        var uid = ++uidCounter;
        canvasBlocks.push({
            uid: uid,
            blockId: block.id,
            name: block.name,
            html: block.html,
            borderRadius: block.border_radius || 0,
            paddingV: block.padding_v || 0,
            paddingH: block.padding_h || 0,
            background: block.background || '',
            gapHeight: 0,
            gapColor: '',
            condition: ''
        });
        renderCanvas();
    }

    function removeBlock(uid) {
        canvasBlocks = canvasBlocks.filter(function(b) { return b.uid !== uid; });
        renderCanvas();
    }

    function moveBlock(uid, direction) {
        var idx = -1;
        for (var i = 0; i < canvasBlocks.length; i++) {
            if (canvasBlocks[i].uid === uid) { idx = i; break; }
        }
        if (idx === -1) return;
        var newIdx = idx + direction;
        if (newIdx < 0 || newIdx >= canvasBlocks.length) return;
        var tmp = canvasBlocks[idx];
        canvasBlocks[idx] = canvasBlocks[newIdx];
        canvasBlocks[newIdx] = tmp;
        renderCanvas();
    }

    function duplicateBlock(uid) {
        var idx = -1;
        for (var i = 0; i < canvasBlocks.length; i++) {
            if (canvasBlocks[i].uid === uid) { idx = i; break; }
        }
        if (idx === -1) return;
        var orig = canvasBlocks[idx];
        var newUid = ++uidCounter;
        canvasBlocks.splice(idx + 1, 0, {
            uid: newUid,
            blockId: orig.blockId,
            name: orig.name,
            html: orig.html,
            borderRadius: orig.borderRadius || 0,
            paddingV: orig.paddingV || 0,
            paddingH: orig.paddingH || 0,
            background: orig.background || '',
            gapHeight: orig.gapHeight || 0,
            gapColor: orig.gapColor || '',
            condition: orig.condition || ''
        });
        renderCanvas();
    }

    function renderCanvas() {
        canvasContainer.innerHTML = '';

        if (canvasBlocks.length === 0) {
            canvasEmpty.style.display = '';
            blockCount.textContent = '0 blocks';
            return;
        }

        canvasEmpty.style.display = 'none';
        blockCount.textContent = canvasBlocks.length + ' block' + (canvasBlocks.length !== 1 ? 's' : '');

        canvasBlocks.forEach(function(item) {
            var el = document.createElement('div');
            el.className = 'builder-block';
            el.setAttribute('data-uid', item.uid);
            el.draggable = true;

            var label = document.createElement('div');
            label.className = 'builder-block-label';
            label.textContent = item.name;
            el.appendChild(label);

            var controls = document.createElement('div');
            controls.className = 'builder-block-controls';

            var btnUp = document.createElement('button');
            btnUp.type = 'button';
            btnUp.textContent = '\u2191';
            btnUp.title = 'Move up';
            btnUp.addEventListener('click', function(e) { e.stopPropagation(); moveBlock(item.uid, -1); });

            var btnDown = document.createElement('button');
            btnDown.type = 'button';
            btnDown.textContent = '\u2193';
            btnDown.title = 'Move down';
            btnDown.addEventListener('click', function(e) { e.stopPropagation(); moveBlock(item.uid, 1); });

            var btnDup = document.createElement('button');
            btnDup.type = 'button';
            btnDup.textContent = '\u2750';
            btnDup.title = 'Duplicate';
            btnDup.addEventListener('click', function(e) { e.stopPropagation(); duplicateBlock(item.uid); });

            var btnDel = document.createElement('button');
            btnDel.type = 'button';
            btnDel.textContent = '\u2715';
            btnDel.title = 'Remove';
            btnDel.style.color = 'var(--error)';
            btnDel.addEventListener('click', function(e) { e.stopPropagation(); removeBlock(item.uid); });

            controls.appendChild(btnUp);
            controls.appendChild(btnDown);
            controls.appendChild(btnDup);
            controls.appendChild(btnDel);
            el.appendChild(controls);

            var preview = document.createElement('div');
            preview.className = 'builder-block-preview';
            var previewHtml = '<table role="presentation" width="600" cellpadding="0" cellspacing="0" border="0" style="background:#ffffff; margin:0 auto;">' +
                previewHTML(item.html) + '</table>';
            preview.innerHTML = previewHtml;
            enableInlineEdit(preview, item);
            el.appendChild(preview);

            var isLast = (canvasBlocks.indexOf(item) === canvasBlocks.length - 1);
            var spacerVisual = null;
            if (!isLast) {
                var gap = document.createElement('div');
                gap.className = 'builder-block-gap';
                gap.style.cssText = 'display:flex; align-items:center; gap:0.5rem; padding:0.5rem 0.75rem; border-top:1px dashed var(--border); font-size:0.8rem; color:var(--text-muted);';
                gap.innerHTML =
                    '<span>Gap below:</span>' +
                    '<input type="number" min="0" max="200" step="2" value="' + (item.gapHeight || 0) + '" style="width:70px; padding:0.2rem 0.4rem;"> px' +
                    '<span style="margin-left:0.5rem;">Color:</span>' +
                    '<input type="color" value="' + (item.gapColor || '#ffffff') + '" style="width:30px; height:24px; padding:0;">' +
                    '<label style="display:flex; align-items:center; gap:0.25rem;"><input type="checkbox"' + (item.gapColor ? ' checked' : '') + '> filled</label>';

                var heightInput = gap.querySelector('input[type="number"]');
                var colorInput = gap.querySelector('input[type="color"]');
                var fillCheckbox = gap.querySelector('input[type="checkbox"]');

                spacerVisual = document.createElement('div');
                spacerVisual.className = 'builder-block-spacer-visual';
                function refreshVisual() {
                    spacerVisual.style.height = (item.gapHeight || 0) + 'px';
                    spacerVisual.style.background = item.gapColor || 'transparent';
                }
                refreshVisual();

                function syncTabs() {
                    if (canvasPreview && canvasPreview.style.display !== 'none') {
                        updatePreview();
                    }
                    if (canvasCode && canvasCode.style.display !== 'none') {
                        updateCode();
                    }
                }

                heightInput.addEventListener('input', function() {
                    item.gapHeight = parseInt(this.value, 10) || 0;
                    refreshVisual();
                    syncTabs();
                });

                colorInput.addEventListener('input', function() {
                    if (!fillCheckbox.checked) fillCheckbox.checked = true;
                    item.gapColor = this.value;
                    refreshVisual();
                    syncTabs();
                });
                fillCheckbox.addEventListener('change', function() {
                    item.gapColor = this.checked ? (colorInput.value || '#75B72B') : '';
                    refreshVisual();
                    syncTabs();
                });

                el.appendChild(gap);
            }

            var condBar = document.createElement('div');
            condBar.style.cssText = 'display:flex; align-items:center; gap:0.5rem; padding:0.4rem 0.75rem; border-top:1px dashed var(--border); font-size:0.8rem; color:var(--text-muted); background:rgba(255,255,255,0.5);';
            var condInputId = 'block-cond-' + item.uid;
            condBar.innerHTML =
                '<label for="' + condInputId + '">Show if:</label>' +
                '<input id="' + condInputId + '" type="text" value="' + (item.condition || '').replace(/"/g, '&quot;') + '"' +
                ' placeholder="VariableName (empty = always)" style="flex:1; padding:0.2rem 0.4rem; font-family:monospace;">' +
                '<span class="text-muted" style="font-size:0.75rem;">renders if {{.X}} is truthy</span>';
            var condInput = condBar.querySelector('input');
            condInput.addEventListener('input', function() {
                item.condition = this.value.trim();
                if (canvasPreview && canvasPreview.style.display !== 'none') updatePreview();
                if (canvasCode && canvasCode.style.display !== 'none') updateCode();
            });
            el.appendChild(condBar);

            el.addEventListener('dragstart', function(e) {
                e.dataTransfer.setData('text/plain', String(item.uid));
                el.classList.add('dragging');
            });
            el.addEventListener('dragend', function() {
                el.classList.remove('dragging');
                clearDropIndicators();
            });
            el.addEventListener('dragover', function(e) {
                e.preventDefault();
                var rect = el.getBoundingClientRect();
                var midY = rect.top + rect.height / 2;
                clearDropIndicators();
                if (e.clientY < midY) {
                    el.style.borderTopColor = 'var(--primary)';
                    el.style.borderTopWidth = '3px';
                } else {
                    el.style.borderBottomColor = 'var(--primary)';
                    el.style.borderBottomWidth = '3px';
                }
            });
            el.addEventListener('dragleave', function() {
                el.style.borderTopColor = '';
                el.style.borderTopWidth = '';
                el.style.borderBottomColor = '';
                el.style.borderBottomWidth = '';
            });
            el.addEventListener('drop', function(e) {
                e.preventDefault();
                clearDropIndicators();
                var draggedUid = parseInt(e.dataTransfer.getData('text/plain'), 10);
                if (isNaN(draggedUid) || draggedUid === item.uid) return;

                var rect = el.getBoundingClientRect();
                var midY = rect.top + rect.height / 2;
                var insertBefore = e.clientY < midY;

                var draggedItem = null;
                canvasBlocks = canvasBlocks.filter(function(b) {
                    if (b.uid === draggedUid) { draggedItem = b; return false; }
                    return true;
                });
                if (!draggedItem) return;

                var targetIdx = -1;
                for (var i = 0; i < canvasBlocks.length; i++) {
                    if (canvasBlocks[i].uid === item.uid) { targetIdx = i; break; }
                }
                if (targetIdx === -1) {
                    canvasBlocks.push(draggedItem);
                } else {
                    canvasBlocks.splice(insertBefore ? targetIdx : targetIdx + 1, 0, draggedItem);
                }
                renderCanvas();
            });

            canvasContainer.appendChild(el);
            if (spacerVisual) {
                canvasContainer.appendChild(spacerVisual);
            }
        });
    }

    function clearDropIndicators() {
        document.querySelectorAll('.builder-block').forEach(function(el) {
            el.style.borderTopColor = '';
            el.style.borderTopWidth = '';
            el.style.borderBottomColor = '';
            el.style.borderBottomWidth = '';
        });
    }

    function makePlaceholderImg(varName, w, h) {
        var svg = '<svg xmlns="http://www.w3.org/2000/svg" width="' + w + '" height="' + h + '">' +
            '<rect width="100%" height="100%" fill="#E4E4E4"/>' +
            '<text x="50%" y="50%" fill="#959595" font-family="Arial" font-size="12" text-anchor="middle" dy=".3em">' +
            varName + '</text></svg>';
        return 'data:image/svg+xml,' + encodeURIComponent(svg);
    }

    function replaceImageVars(html) {
        return html.replace(/(<img[^>]*src=")(\{\{\.\w+\}\})("[^>]*>)/gi, function(match, before, varExpr, after) {
            var varName = varExpr.replace(/\{\{\.|\}\}/g, '');
            var wMatch = match.match(/width[=:]["']?(\d+)/i);
            var hMatch = match.match(/height[=:]["']?(\d+)/i);
            var w = wMatch ? wMatch[1] : '200';
            var h = hMatch ? hMatch[1] : '80';
            return before + makePlaceholderImg(varName, w, h) + after;
        });
    }

    function replaceTextVars(html) {
        return html.replace(/\{\{\.(\w+)\}\}/g, function(match, varName) {
            return '<span data-placeholder="' + varName + '" style="background:#E4E4E4;color:#959595;padding:1px 4px;border-radius:3px;font-size:12px;">' + varName + '</span>';
        });
    }

    function canonicalText(node) {
        var parts = [];
        node.childNodes.forEach(function(c) {
            if (c.nodeType === 3) parts.push(c.nodeValue);
            else if (c.nodeType === 1) {
                if (c.dataset && c.dataset.placeholder) parts.push('{{.' + c.dataset.placeholder + '}}');
                else if (c.tagName === 'BR') parts.push('\n');
                else parts.push(c.textContent);
            }
        });
        return parts.join('');
    }

    function applyTestValues(html) {
        var testValues = JSON.parse(localStorage.getItem('sendry_test_values') || '{}');
        for (var key in testValues) {
            if (testValues[key]) {
                html = html.split('{{.' + key + '}}').join(testValues[key]);
            }
        }
        return html;
    }

    function previewHTML(html) {
        html = applyTestValues(html);
        html = replaceImageVars(html);
        html = replaceTextVars(html);
        return html;
    }

    var inlineEditTimers = {};

    if (!window.__inlineEditGuard) {
        window.__inlineEditGuard = true;
        document.addEventListener('mousedown', function(e) {
            var t = e.target;
            if (t && t.nodeType === 1 && t.closest && t.closest('[contenteditable="true"]')) {
                var blockEl = t.closest('.builder-block');
                if (blockEl) blockEl.draggable = false;
                e.stopPropagation();
            }
        }, true);
        document.addEventListener('focusout', function(e) {
            var t = e.target;
            if (t && t.getAttribute && t.getAttribute('contenteditable') === 'true') {
                var blockEl = t.closest && t.closest('.builder-block');
                if (blockEl) blockEl.draggable = true;
            }
        }, true);
    }

    function enableInlineEdit(previewEl, item) {
        previewEl.querySelectorAll('td, th, span, p, h1, h2, h3, h4, h5, h6, li, a, b, strong, i, em').forEach(function(node) {
            if (node.dataset && node.dataset.placeholder) return;
            var blocksParent = false;
            for (var c = 0; c < node.children.length; c++) {
                var ch = node.children[c];
                if (ch.tagName === 'BR') continue;
                if (ch.dataset && ch.dataset.placeholder) continue;
                blocksParent = true;
                break;
            }
            if (blocksParent) return;
            var canonical = canonicalText(node);
            var trimmed = canonical.trim();
            if (!trimmed) return;
            if (item.html.indexOf(trimmed) === -1) return;
            node.setAttribute('contenteditable', 'true');
            node.setAttribute('spellcheck', 'false');
            node.dataset.originalText = trimmed;
            node.addEventListener('mousedown', function(e) { e.stopPropagation(); });
            node.addEventListener('focus', function() {
                var blockEl = node.closest('.builder-block');
                if (blockEl) blockEl.draggable = false;
            });
            node.addEventListener('input', function() { scheduleInlineSave(item, previewEl); });
            node.addEventListener('blur', function() {
                var blockEl = node.closest('.builder-block');
                if (blockEl) blockEl.draggable = true;
                scheduleInlineSave(item, previewEl, true);
            });
            node.addEventListener('keydown', function(e) {
                if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); node.blur(); }
            });
        });
    }

    function scheduleInlineSave(item, previewEl, immediate) {
        clearTimeout(inlineEditTimers[item.uid]);
        var delay = immediate ? 0 : 700;
        console.log('[inline-edit] scheduled', {uid: item.uid, blockId: item.blockId, immediate: !!immediate, delay: delay});
        inlineEditTimers[item.uid] = setTimeout(function() { saveInlineEdit(item, previewEl); }, delay);
    }

    function saveInlineEdit(item, previewEl) {
        var newSourceHTML = item.html;
        var changes = 0;
        var diagnostics = [];
        previewEl.querySelectorAll('[contenteditable="true"]').forEach(function(node) {
            var original = node.dataset.originalText;
            var current = canonicalText(node).trim();
            if (original === undefined || original === current) return;
            var origPh = (original.match(/\{\{/g) || []).length;
            var curPh = (current.match(/\{\{/g) || []).length;
            if (origPh !== curPh) {
                diagnostics.push('placeholder count mismatch in ' + JSON.stringify(original.slice(0, 30)));
                return;
            }
            var idx = newSourceHTML.indexOf(original);
            if (idx === -1) {
                diagnostics.push('original text not found in source: ' + JSON.stringify(original.slice(0, 30)));
                return;
            }
            newSourceHTML = newSourceHTML.slice(0, idx) + current + newSourceHTML.slice(idx + original.length);
            node.dataset.originalText = current;
            changes++;
        });
        console.log('[inline-edit] save called', {
            blockId: item.blockId,
            sourceLen: item.html.length,
            newSourceLen: newSourceHTML.length,
            changes: changes,
            diagnostics: diagnostics
        });
        if (changes === 0 || newSourceHTML === item.html) {
            console.log('[inline-edit] no changes detected');
            return;
        }
        console.log('[inline-edit] POST /blocks/' + item.blockId + '/inline-edit');
        fetch('/blocks/' + item.blockId + '/inline-edit', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            credentials: 'same-origin',
            body: JSON.stringify({html: newSourceHTML})
        }).then(function(r) {
            console.log('[inline-edit] response status:', r.status);
            if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
            item.html = newSourceHTML;
            if (blockMap[item.blockId]) blockMap[item.blockId].html = newSourceHTML;
            console.log('[inline-edit] saved OK');
        }).catch(function(err) {
            console.error('[inline-edit] failed:', err.message);
            alert('Не удалось сохранить: ' + err.message);
            renderCanvas();
        });
    }

    function renderedBackToSource(sourceHTML, renderedHTML) {
        var wrap = function(html) { return '<table><tbody>' + html + '</tbody></table>'; };
        var srcDoc = new DOMParser().parseFromString(wrap(sourceHTML), 'text/html');
        var rndDoc = new DOMParser().parseFromString(wrap(renderedHTML), 'text/html');
        var srcRoot = srcDoc.querySelector('tbody');
        var rndRoot = rndDoc.querySelector('tbody');
        if (!srcRoot || !rndRoot) return null;
        if (!walkAndPatch(srcRoot, rndRoot)) return null;
        return srcRoot.innerHTML;
    }

    function walkAndPatch(src, rnd) {
        if (src.nodeType === 3 && rnd.nodeType === 3) {
            var srcText = src.nodeValue;
            var rndText = rnd.nodeValue;
            if (srcText === rndText) return true;
            var phMatches = srcText.match(/\{\{[^}]+\}\}/g);
            if (!phMatches) {
                src.nodeValue = rndText;
                return true;
            }
            if (phMatches.length > 1) return true;
            var ph = phMatches[0];
            var idx = srcText.indexOf(ph);
            var srcBefore = srcText.slice(0, idx);
            var srcAfter = srcText.slice(idx + ph.length);
            var newBefore = srcBefore;
            var newAfter = srcAfter;
            if (srcAfter.length > 0 && rndText.endsWith(srcAfter)) {
                var rndBeforeWithVal = rndText.slice(0, rndText.length - srcAfter.length);
                var phValueLen = rndBeforeWithVal.length - srcBefore.length;
                if (phValueLen >= 0 && phValueLen <= rndBeforeWithVal.length) {
                    newBefore = rndBeforeWithVal.slice(0, rndBeforeWithVal.length - phValueLen);
                } else {
                    newBefore = rndBeforeWithVal;
                }
            } else if (srcBefore.length > 0 && rndText.startsWith(srcBefore)) {
                var rndValueWithAfter = rndText.slice(srcBefore.length);
                if (rndValueWithAfter.length >= srcAfter.length) {
                    newAfter = rndValueWithAfter.slice(rndValueWithAfter.length - srcAfter.length);
                } else {
                    newAfter = '';
                }
            } else {
                if (srcBefore.length === 0) {
                    newBefore = '';
                    newAfter = rndText;
                } else if (srcAfter.length === 0) {
                    newBefore = rndText;
                    newAfter = '';
                } else {
                    newBefore = rndText;
                    newAfter = '';
                }
            }
            src.nodeValue = newBefore + ph + newAfter;
            return true;
        }
        var srcChildren = src.childNodes;
        var rndChildren = rnd.childNodes;
        var sLen = srcChildren.length;
        var rLen = rndChildren.length;
        if (sLen !== rLen) {
            return false;
        }
        for (var i = 0; i < sLen; i++) {
            if (!walkAndPatch(srcChildren[i], rndChildren[i])) return false;
        }
        return true;
    }

    function spacerRow(height, color) {
        var bgAttr = '', bgInline = '';
        if (color) {
            bgAttr = ' bgcolor="' + color + '"';
            bgInline = 'background-color:' + color + ';';
        }
        return '<tr><td height="' + height + '"' + bgAttr +
            ' style="font-size:0;line-height:0;height:' + height + 'px;' + bgInline + '">&nbsp;</td></tr>';
    }

    function readContainerSettings() {
        var radiusEl = document.getElementById('container-radius');
        var radiusTopEl = document.getElementById('container-radius-top');
        var radiusBotEl = document.getElementById('container-radius-bottom');
        var transEl = document.getElementById('container-transparent');
        var widthEl = document.getElementById('container-width');
        var padVEl = document.getElementById('container-padding-v');
        var padHEl = document.getElementById('container-padding-h');
        var pageBgEl = document.getElementById('page-background');
        var pageBgTransEl = document.getElementById('page-background-transparent');
        var radius = radiusEl ? parseInt(radiusEl.value, 10) : 8;
        if (isNaN(radius) || radius < 0) radius = 0;
        var radiusTop = radiusTopEl ? parseInt(radiusTopEl.value, 10) : 0;
        if (isNaN(radiusTop) || radiusTop < 0) radiusTop = 0;
        var radiusBot = radiusBotEl ? parseInt(radiusBotEl.value, 10) : 0;
        if (isNaN(radiusBot) || radiusBot < 0) radiusBot = 0;
        var width = widthEl ? parseInt(widthEl.value, 10) : 600;
        if (isNaN(width) || width < 320) width = 600;
        var padV = padVEl ? parseInt(padVEl.value, 10) : 20;
        if (isNaN(padV) || padV < 0) padV = 0;
        var padH = padHEl ? parseInt(padHEl.value, 10) : 0;
        if (isNaN(padH) || padH < 0) padH = 0;
        var transparent = !!(transEl && transEl.checked);
        var pageBG = (pageBgEl && pageBgEl.value) ? pageBgEl.value : '#F5F5F5';
        if (pageBgTransEl && pageBgTransEl.checked) pageBG = 'transparent';
        return {
            radius: radius,
            radiusTop: radiusTop,
            radiusBot: radiusBot,
            transparent: transparent,
            width: width,
            paddingV: padV,
            paddingH: padH,
            pageBG: pageBG
        };
    }

    function wrapBlockShell(inner, radius, padV, padH, background) {
        radius = radius || 0; padV = padV || 0; padH = padH || 0;
        var bg = (background || '').trim();
        if (!bg && radius > 0) bg = '#FEFFFE';
        if (radius <= 0 && padV <= 0 && padH <= 0 && !bg) return inner;
        var tableStyles = ['width:100%', 'border-collapse:separate'];
        if (radius > 0) tableStyles.push('border-radius:' + radius + 'px', 'overflow:hidden');
        if (bg) tableStyles.push('background-color:' + bg);
        var tableStyle = tableStyles.join(';') + ';';
        var contentTdStyle = 'padding:' + padV + 'px ' + padH + 'px;';
        var trimmed = inner.replace(/^[\s]+/, '').toLowerCase();
        if (trimmed.indexOf('<table') === 0) {
            return '<tr><td style="padding:0;"><table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="' + tableStyle + '"><tr><td style="' + contentTdStyle + '">' + inner + '</td></tr></table></td></tr>';
        }
        return '<tr><td style="padding:0;"><table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="' + tableStyle + '"><tr><td style="' + contentTdStyle + '"><table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="border-collapse:collapse;width:100%;">' + inner + '</table></td></tr></table></td></tr>';
    }

    function applyWrapper(blocksHtml, settings) {
        var containerBG = settings.transparent ? 'transparent' : '#FEFFFE';
        var top = settings.radiusTop > 0 ? settings.radiusTop : settings.radius;
        var bot = settings.radiusBot > 0 ? settings.radiusBot : settings.radius;
        var radiusShorthand = top + 'px ' + top + 'px ' + bot + 'px ' + bot + 'px';
        var html = wrapper
            .split('{{CONTAINER_RADIUS}}').join(radiusShorthand)
            .split('{{CONTAINER_WIDTH}}').join(String(settings.width))
            .split('{{CONTAINER_PADDING_V}}').join(String(settings.paddingV))
            .split('{{CONTAINER_PADDING_H}}').join(String(settings.paddingH))
            .split('{{CONTAINER_BG}}').join(containerBG)
            .split('{{PAGE_BG}}').join(settings.pageBG || '#F5F5F5');
        var placeholder = '{{BLOCKS}}';
        var idx = html.indexOf(placeholder);
        if (idx !== -1) {
            return html.substring(0, idx) + blocksHtml + html.substring(idx + placeholder.length);
        }
        return blocksHtml;
    }

    function assembleHTML() {
        if (canvasBlocks.length === 0) return '';

        var settings = readContainerSettings();
        var parts = [];
        canvasBlocks.forEach(function(b, i) {
            var cond = (b.condition || '').trim();
            if (cond) parts.push('{{if .' + cond + '}}');
            parts.push(wrapBlockShell(b.html, b.borderRadius || 0, b.paddingV || 0, b.paddingH || 0, b.background || ''));
            var isLast = (i === canvasBlocks.length - 1);
            if (!isLast && b.gapHeight > 0) {
                parts.push(spacerRow(b.gapHeight, b.gapColor || ''));
            }
            if (cond) parts.push('{{end}}');
        });
        return applyWrapper(parts.join('\n'), settings);
    }

    function extractVariables() {
        var html = assembleHTML();
        return JSON.stringify(buildSkeleton(html), null, 2);
    }

    var KEYWORDS = {end:1, else:1, with:1, range:1, if:1, block:1, define:1, template:1};

    function sampleValueFor(name) {
        var low = name.toLowerCase();
        if (low.indexOf('email') !== -1) return 'user@example.com';
        if (low.indexOf('url') !== -1 || low.indexOf('link') !== -1) return 'https://example.com';
        if (low.indexOf('phone') !== -1) return '+7 999 123-45-67';
        if (low.indexOf('year') !== -1) return '2026';
        if (low.indexOf('date') !== -1) return '2026-01-01';
        if (low.indexOf('price') !== -1 || low.indexOf('amount') !== -1 || low.indexOf('total') !== -1 || low.indexOf('sum') !== -1) return '100 ₽';
        if (low.indexOf('percent') !== -1) return '10%';
        if (low.indexOf('quantity') !== -1 || low.indexOf('count') !== -1) return '1';
        if (low.indexOf('number') !== -1 || /id$/.test(low)) return '12345';
        return 'Sample ' + name;
    }

    function buildSkeleton(body) {
        var out = {};
        collectVars(body, out);
        return out;
    }

    var IDENT = '[A-Za-z_][A-Za-z0-9_]*';
    var IDENT_PATH = IDENT + '(?:\\.' + IDENT + ')*';

    function setNestedKey(out, path, value) {
        var parts = path.split('.');
        var cur = out;
        for (var i = 0; i < parts.length - 1; i++) {
            var p = parts[i];
            if (typeof cur[p] !== 'object' || cur[p] === null || Array.isArray(cur[p])) {
                cur[p] = {};
            }
            cur = cur[p];
        }
        var leaf = parts[parts.length - 1];
        if (!(leaf in cur)) cur[leaf] = value;
    }

    function lastSegment(path) {
        var i = path.lastIndexOf('.');
        return i >= 0 ? path.slice(i + 1) : path;
    }

    function collectVars(body, out) {
        var stripped = stripTopLevelRangesJS(body, out);
        var ifRE = new RegExp('\\{\\{\\s*if\\s+\\.?(' + IDENT_PATH + ')\\s*\\}\\}', 'g');
        var m;
        while ((m = ifRE.exec(stripped)) !== null) {
            if (KEYWORDS[m[1]]) continue;
            setNestedKey(out, m[1], sampleValueFor(lastSegment(m[1])));
        }
        var varRE = new RegExp('\\{\\{\\s*\\.?(' + IDENT_PATH + ')\\s*\\}\\}', 'g');
        while ((m = varRE.exec(stripped)) !== null) {
            if (KEYWORDS[m[1]]) continue;
            setNestedKey(out, m[1], sampleValueFor(lastSegment(m[1])));
        }
    }

    function stripTopLevelRangesJS(body, out) {
        var rangeOpenRE = new RegExp('^\\{\\{\\s*range\\s+\\.?(' + IDENT_PATH + ')\\s*\\}\\}$');
        var anyOpenRE  = /\{\{\s*(range|if|with|block|define)\b[^}]*\}\}/g;
        var endRE      = /\{\{\s*end\s*\}\}/g;
        var sb = '';
        var i = 0;
        while (i < body.length) {
            var open = body.indexOf('{{', i);
            if (open < 0) { sb += body.slice(i); break; }
            sb += body.slice(i, open);
            var close = body.indexOf('}}', open);
            if (close < 0) { sb += body.slice(open); break; }
            close += 2;
            var action = body.slice(open, close);
            var rm = action.match(rangeOpenRE);
            if (!rm) { sb += action; i = close; continue; }
            var depth = 1, pos = close;
            while (pos < body.length) {
                anyOpenRE.lastIndex = pos;
                endRE.lastIndex = pos;
                var nextOpen = anyOpenRE.exec(body);
                var nextEnd  = endRE.exec(body);
                if (!nextEnd) { depth = -1; break; }
                if (nextOpen && nextOpen.index < nextEnd.index) {
                    depth++;
                    pos = nextOpen.index + nextOpen[0].length;
                } else {
                    depth--;
                    if (depth === 0) {
                        var inner = body.slice(close, nextEnd.index);
                        var element = {};
                        collectVars(inner, element);
                        setNestedKey(out, rm[1], [element]);
                        i = nextEnd.index + nextEnd[0].length;
                        break;
                    }
                    pos = nextEnd.index + nextEnd[0].length;
                }
            }
            if (depth !== 0) { sb += action; i = close; }
        }
        return sb;
    }

    function updatePreview() {
        var html = assembleHTML();
        if (!html) {
            previewFrame.innerHTML = '<div style="padding:40px; text-align:center; color:#999;">Add blocks to see preview</div>';
            return;
        }

        previewFrame.innerHTML = '<div style="padding:40px; text-align:center; color:#999;">Rendering…</div>';

        var data = Object.assign({}, builderSampleData());
        var testValues = JSON.parse(localStorage.getItem('sendry_test_values') || '{}');
        Object.assign(data, testValues);
        var dataTa = document.getElementById('builder-preview-data');
        var statusEl = document.getElementById('builder-preview-status');
        if (statusEl) { statusEl.textContent = ''; statusEl.style.color = ''; }
        if (dataTa && dataTa.value.trim()) {
            try {
                Object.assign(data, JSON.parse(dataTa.value));
            } catch (e) {
                if (statusEl) {
                    statusEl.textContent = 'JSON parse error: ' + e.message;
                    statusEl.style.color = 'crimson';
                }
                previewFrame.innerHTML = '<pre style="padding:1rem; color:crimson;">JSON parse error: ' + e.message + '</pre>';
                return;
            }
        }

        fetch('/builder/render-preview', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            credentials: 'same-origin',
            body: JSON.stringify({html: html, data: data})
        }).then(function(r) {
            if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
            return r.text();
        }).then(function(rendered) {
            previewFrame.innerHTML = '';
            var iframe = document.createElement('iframe');
            var frameWidth = previewFrame.style.width || '600px';
            iframe.style.width = frameWidth;
            iframe.style.border = 'none';
            iframe.style.minHeight = '400px';
            iframe.style.display = 'block';
            iframe.style.margin = '0 auto';
            previewFrame.appendChild(iframe);

            var doc = iframe.contentDocument || iframe.contentWindow.document;
            doc.open();
            doc.write(rendered);
            doc.close();

            setTimeout(function() {
                try { iframe.style.height = doc.documentElement.scrollHeight + 'px'; }
                catch(e) {}
            }, 100);
            if (statusEl) {
                statusEl.textContent = 'Rendered';
                statusEl.style.color = 'var(--text-muted)';
            }
        }).catch(function(err) {
            previewFrame.innerHTML = '<pre style="padding:1rem; color:crimson; white-space:pre-wrap;">' + err.message + '</pre>';
            if (statusEl) {
                statusEl.textContent = 'Error';
                statusEl.style.color = 'crimson';
            }
        });
    }

    function builderSampleData() {
        return buildSkeleton(assembleHTML());
    }

    function updateCode() {
        codeOutput.value = assembleHTML();
    }

    var btnPreviewRender = document.getElementById('btn-preview-render');
    if (btnPreviewRender) {
        btnPreviewRender.addEventListener('click', updatePreview);
    }

    ['container-radius', 'container-radius-top', 'container-radius-bottom', 'container-transparent', 'container-width', 'container-padding-v', 'container-padding-h', 'page-background', 'page-background-transparent'].forEach(function(id) {
        var el = document.getElementById(id);
        if (!el) return;
        var evt = (el.type === 'checkbox') ? 'change' : 'input';
        el.addEventListener(evt, function() {
            if (canvasPreview && canvasPreview.style.display !== 'none') updatePreview();
            if (canvasCode && canvasCode.style.display !== 'none') updateCode();
        });
    });

    document.getElementById('btn-clear').addEventListener('click', function() {
        if (canvasBlocks.length === 0) return;
        if (!confirm('Remove all blocks?')) return;
        canvasBlocks = [];
        renderCanvas();
    });

    form.addEventListener('submit', function(e) {
        var html = assembleHTML();
        if (!html) {
            e.preventDefault();
            alert('Please add at least one block to the email.');
            return;
        }
        htmlHidden.value = html;
        variablesHidden.value = extractVariables();
        if (blockRefsHidden) {
            blockRefsHidden.value = JSON.stringify(canvasBlocks.map(function(b) {
                return {
                    block_id: b.blockId,
                    gap_height: b.gapHeight || 0,
                    gap_color: b.gapColor || '',
                    condition: (b.condition || '').trim()
                };
            }));
        }
    });
})();
