// Sendry Web Application JavaScript

(function() {
    'use strict';

    // Filter forms apply as soon as a value changes; the submit button is only a no-JS fallback
    function initAutoFilters(root) {
        (root || document).querySelectorAll('form.filter-form[method="get" i]').forEach(function(form) {
            if (form.dataset.autoFilter) return;
            form.dataset.autoFilter = '1';
            var submit = function() {
                if (form.requestSubmit) form.requestSubmit(); else form.submit();
            };
            form.querySelectorAll('select').forEach(function(sel) {
                sel.addEventListener('change', submit);
            });
            form.querySelectorAll('input[type="text"], input[type="search"]').forEach(function(input) {
                var initial = input.value;
                input.addEventListener('change', function() {
                    if (input.value !== initial) submit();
                });
            });
            form.querySelectorAll('button[type="submit"], button:not([type])').forEach(function(btn) {
                btn.style.display = 'none';
            });
        });
    }

    // Partial reload: a [data-partial] region (filter form, pagination, results)
    // is refetched and swapped in place instead of reloading the whole page.
    // Links elsewhere on the page that point at it (href ending in #id) swap it too.
    // Without JS the forms and links still work as plain navigation to the anchor.
    function initPartials() {
        var regions = document.querySelectorAll('[data-partial][id]');
        if (!regions.length) return;
        var path = location.pathname;
        var pending = null;
        var pushed = false; // history entries exist only after the first swap

        function regionFor(el, url) {
            var inside = el.closest('[data-partial][id]');
            if (inside) return inside;
            var target = url.hash && document.getElementById(url.hash.slice(1));
            return target && target.hasAttribute('data-partial') ? target : null;
        }

        function load(region, url, opts) {
            if (pending) pending.abort();
            pending = new AbortController();
            region.classList.add('is-loading');
            fetch(url.pathname + url.search, { signal: pending.signal })
                .then(function(r) {
                    if (!r.ok) throw new Error('HTTP ' + r.status);
                    return r.text();
                })
                .then(function(html) {
                    var doc = new DOMParser().parseFromString(html, 'text/html');
                    var fresh = doc.getElementById(region.id);
                    if (!fresh) throw new Error('no #' + region.id + ' in response');
                    // keep keyboard focus on the same control (e.g. an auto-submitted filter)
                    var focused = document.activeElement;
                    var focusSel = null;
                    if (focused && region.contains(focused)) {
                        if (focused.id) focusSel = '#' + CSS.escape(focused.id);
                        else if (focused.name) focusSel = focused.tagName.toLowerCase() + '[name="' + CSS.escape(focused.name) + '"]';
                    }
                    region.replaceWith(fresh);
                    if (focusSel) {
                        var again = fresh.querySelector(focusSel);
                        if (again) again.focus({ preventScroll: true });
                    }
                    I18n.apply();
                    initAutoFilters(fresh);
                    if (opts.push) {
                        history.pushState({ partial: fresh.id }, '', url.pathname + url.search + '#' + fresh.id);
                        pushed = true;
                    }
                    if (opts.scroll) {
                        var top = fresh.getBoundingClientRect().top;
                        if (top < 0 || top > window.innerHeight / 2) fresh.scrollIntoView({ behavior: 'smooth' });
                    }
                })
                .catch(function(err) {
                    if (err.name === 'AbortError') return;
                    location.href = url.href; // fall back to a full page load
                });
        }

        document.addEventListener('submit', function(e) {
            var form = e.target;
            if (!(form.method || '').match(/^get$/i)) return;
            var url = new URL(form.action, location.href);
            if (url.pathname !== path) return;
            var region = regionFor(form, url);
            if (!region) return;
            e.preventDefault();
            url.search = new URLSearchParams(new FormData(form)).toString();
            load(region, url, { push: true });
        });

        document.addEventListener('click', function(e) {
            if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
            var a = e.target.closest('a[href]');
            // getAttribute: on SVG links (donut segments) href and target are not strings
            if (!a || a.getAttribute('target')) return;
            var url = new URL(a.getAttribute('href'), location.href);
            if (url.origin !== location.origin || url.pathname !== path) return;
            var region = regionFor(a, url);
            if (!region) return;
            e.preventDefault();
            load(region, url, { push: true, scroll: !a.closest('[data-partial]') || a.closest('.pagination') });
        });

        window.addEventListener('popstate', function() {
            if (!pushed) return;
            var region = document.querySelector('[data-partial][id]');
            if (region) load(region, new URL(location.href), {});
        });
    }

    // Desktop sidebar: collapse to icons, remembered per browser
    function initSidebarCollapse() {
        var btn = document.getElementById('sidebar-collapse');
        if (!btn) return;
        var root = document.documentElement;

        function apply(collapsed) {
            if (collapsed) root.setAttribute('data-nav', 'collapsed'); else root.removeAttribute('data-nav');
            btn.setAttribute('aria-expanded', collapsed ? 'false' : 'true');
            var key = collapsed ? 'nav.expand' : 'nav.collapse';
            btn.setAttribute('data-i18n-title', key);
            btn.title = I18n.t(key);
            btn.setAttribute('aria-label', btn.title);
            // Tooltips stand in for the hidden labels
            document.querySelectorAll('.sidebar .nav-item').forEach(function(item) {
                var label = item.querySelector('.nav-label');
                if (!label) return;
                if (collapsed) item.title = label.textContent.trim(); else item.removeAttribute('title');
            });
        }

        btn.addEventListener('click', function() {
            var collapsed = !root.hasAttribute('data-nav');
            localStorage.setItem('navCollapsed', collapsed ? '1' : '0');
            apply(collapsed);
        });
        document.querySelectorAll('.lang-btn').forEach(function(b) {
            b.addEventListener('click', function() { apply(root.hasAttribute('data-nav')); });
        });
        apply(root.hasAttribute('data-nav'));
    }

    // Theme Toggle
    function initThemeToggle() {
        var toggle = document.getElementById('theme-toggle');
        if (!toggle) return;

        var currentTheme = localStorage.getItem('theme') || 'light';
        updateThemeIcon(currentTheme);

        toggle.addEventListener('click', function() {
            var theme = document.documentElement.getAttribute('data-theme');
            var newTheme = theme === 'light' ? 'dark' : 'light';

            document.documentElement.setAttribute('data-theme', newTheme);
            localStorage.setItem('theme', newTheme);
            updateThemeIcon(newTheme);
        });
    }

    // Icons are shown/hidden by CSS based on [data-theme]
    function updateThemeIcon(theme) {
        var toggle = document.getElementById('theme-toggle');
        if (toggle) toggle.setAttribute('aria-pressed', theme === 'dark' ? 'true' : 'false');
    }

    // Mobile off-canvas navigation
    function initNavToggle() {
        var toggle = document.getElementById('nav-toggle');
        if (!toggle) return;
        var backdrop = document.getElementById('nav-backdrop');

        function setOpen(open) {
            document.body.classList.toggle('nav-open', open);
            toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
        }

        toggle.addEventListener('click', function() {
            setOpen(!document.body.classList.contains('nav-open'));
        });
        if (backdrop) backdrop.addEventListener('click', function() { setOpen(false); });
        document.addEventListener('keydown', function(e) {
            if (e.key === 'Escape' && document.body.classList.contains('nav-open')) {
                setOpen(false);
                toggle.focus();
            }
        });
        window.addEventListener('resize', function() {
            if (window.innerWidth >= 900) setOpen(false);
        });
    }

    // Confirm dialogs
    function initConfirmDialogs() {
        document.addEventListener('click', function(e) {
            var target = e.target.closest('[data-confirm]');
            if (target) {
                var message = target.getAttribute('data-confirm') || I18n.t('confirm_delete');
                if (!confirm(message)) {
                    e.preventDefault();
                    e.stopPropagation();
                }
            }
        });
    }

    // Success flash messages auto-hide. Errors, warnings and blocks marked
    // alert-persistent (e.g. a new API key shown once) stay.
    function initFlashMessages() {
        var alerts = document.querySelectorAll('.alert-success:not(.alert-persistent)');
        alerts.forEach(function(alert) {
            setTimeout(function() {
                alert.style.opacity = '0';
                alert.style.transition = 'opacity 0.3s';
                setTimeout(function() {
                    alert.remove();
                }, 300);
            }, 5000);
        });
    }

    // Initialize everything
    function init() {
        initThemeToggle();
        initNavToggle();
        initSidebarCollapse();
        initAutoFilters();
        initPartials();
        initConfirmDialogs();
        initFlashMessages();
    }

    // Run on DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
