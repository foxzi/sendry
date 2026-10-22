// Sendry Web Application JavaScript

(function() {
    'use strict';

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

    function updateThemeIcon(theme) {
        var sunIcon = document.querySelector('.icon-sun');
        var moonIcon = document.querySelector('.icon-moon');

        if (sunIcon && moonIcon) {
            if (theme === 'dark') {
                sunIcon.style.display = 'inline';
                moonIcon.style.display = 'none';
            } else {
                sunIcon.style.display = 'none';
                moonIcon.style.display = 'inline';
            }
        }
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
