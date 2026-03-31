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

    var canvasContainer = document.getElementById('canvas-blocks');
    var canvasEmpty = document.getElementById('canvas-empty');
    var blockCount = document.getElementById('block-count');
    var htmlHidden = document.getElementById('html-hidden');
    var variablesHidden = document.getElementById('variables-hidden');
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

            if (target === 'preview') updatePreview();
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
            html: block.html
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
            html: orig.html
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
            el.appendChild(preview);

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
            return '<span style="background:#E4E4E4;color:#959595;padding:1px 4px;border-radius:3px;font-size:12px;">' + varName + '</span>';
        });
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

    function assembleHTML() {
        if (canvasBlocks.length === 0) return '';

        var blocksHtml = canvasBlocks.map(function(b) { return b.html; }).join('\n');

        var placeholder = '{{BLOCKS}}';
        var idx = wrapper.indexOf(placeholder);
        if (idx !== -1) {
            return wrapper.substring(0, idx) + blocksHtml + wrapper.substring(idx + placeholder.length);
        }
        return blocksHtml;
    }

    function extractVariables() {
        var html = assembleHTML();
        var vars = {};
        var regex = /\{\{\.(\w+)\}\}/g;
        var match;
        while ((match = regex.exec(html)) !== null) {
            vars[match[1]] = '';
        }
        return JSON.stringify(vars, null, 2);
    }

    function updatePreview() {
        var html = assembleHTML();
        if (!html) {
            previewFrame.innerHTML = '<div style="padding:40px; text-align:center; color:#999;">Add blocks to see preview</div>';
            return;
        }
        html = previewHTML(html);

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
        doc.write(html);
        doc.close();

        setTimeout(function() {
            try {
                iframe.style.height = doc.documentElement.scrollHeight + 'px';
            } catch(e) {}
        }, 100);
    }

    function updateCode() {
        codeOutput.value = assembleHTML();
    }

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
    });
})();
