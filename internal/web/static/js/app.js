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

    // Timezone handling
    var Timezone = {
        current: localStorage.getItem('timezone') || Intl.DateTimeFormat().resolvedOptions().timeZone,

        set: function(tz) {
            this.current = tz;
            localStorage.setItem('timezone', tz);
            this.formatAllDates();
        },

        get: function() {
            return this.current;
        },

        format: function(dateStr, options) {
            if (!dateStr) return '';

            var date = new Date(dateStr);
            if (isNaN(date.getTime())) return dateStr;

            var defaultOptions = {
                timeZone: this.current,
                year: 'numeric',
                month: 'short',
                day: 'numeric',
                hour: '2-digit',
                minute: '2-digit'
            };

            options = Object.assign({}, defaultOptions, options || {});

            try {
                return new Intl.DateTimeFormat(I18n.getLang(), options).format(date);
            } catch (e) {
                return date.toLocaleString();
            }
        },

        formatRelative: function(dateStr) {
            if (!dateStr) return '';

            var date = new Date(dateStr);
            if (isNaN(date.getTime())) return dateStr;

            var now = new Date();
            var diff = now - date;
            var minutes = Math.floor(diff / 60000);
            var hours = Math.floor(diff / 3600000);
            var days = Math.floor(diff / 86400000);

            if (minutes < 1) return I18n.t('just_now');
            if (minutes < 60) return I18n.t('minutes_ago', {n: minutes});
            if (hours < 24) return I18n.t('hours_ago', {n: hours});
            if (days < 30) return I18n.t('days_ago', {n: days});

            return this.format(dateStr);
        },

        formatAllDates: function() {
            document.querySelectorAll('[data-datetime]').forEach(function(el) {
                var dateStr = el.getAttribute('data-datetime');
                var format = el.getAttribute('data-format') || 'full';

                if (format === 'relative') {
                    el.textContent = Timezone.formatRelative(dateStr);
                } else {
                    el.textContent = Timezone.format(dateStr);
                }
            });
        },

        getCommonTimezones: function() {
            return [
                'UTC',
                'Europe/Moscow',
                'Europe/London',
                'Europe/Paris',
                'Europe/Berlin',
                'America/New_York',
                'America/Chicago',
                'America/Denver',
                'America/Los_Angeles',
                'Asia/Tokyo',
                'Asia/Shanghai',
                'Asia/Singapore',
                'Australia/Sydney'
            ];
        }
    };

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

    // Flash messages auto-hide
    function initFlashMessages() {
        var alerts = document.querySelectorAll('.alert:not(.alert-error)');
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

    // HTMX event handlers
    function initHTMX() {
        document.body.addEventListener('htmx:afterSwap', function(e) {
            // Re-apply translations after HTMX swap
            I18n.init();
            // Re-format dates
            Timezone.formatAllDates();
        });

        document.body.addEventListener('htmx:responseError', function(e) {
            console.error('HTMX error:', e.detail);
        });
    }

    // Initialize everything
    function init() {
        initThemeToggle();
        initConfirmDialogs();
        initFlashMessages();
        initHTMX();
        Timezone.formatAllDates();
    }

    // Export to global scope
    window.Timezone = Timezone;

    // Run on DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
