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
