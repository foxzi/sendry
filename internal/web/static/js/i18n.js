// Internationalization support
var I18n = (function() {
    var translations = {
        en: {
            // Navigation
            'nav.overview': 'Overview',
            'nav.campaigns': 'Campaigns',
            'nav.templates': 'Templates',
            'nav.audience': 'Audience',
            'nav.sends': 'API sends',
            'nav.infra': 'Infrastructure',
            'nav.admin': 'Administration',
            'nav.group_email': 'Email',
            'nav.group_system': 'System',
            'nav.collapse': 'Collapse menu',
            'nav.expand': 'Expand menu',
            'nav.section_pages': 'Section pages',
            'nav.queue_badge': 'Messages waiting in the queue',
            'sub.campaigns': 'Campaigns',
            'sub.runs': 'Runs',
            'sub.templates': 'Templates',
            'sub.blocks': 'Blocks',
            'sub.media': 'Media',
            'sub.variables': 'Variables',
            'sub.servers': 'Servers',
            'sub.queue': 'Queue',
            'sub.domains': 'Domains',
            'sub.dkim': 'DKIM',
            'sub.monitoring': 'Monitoring',
            'sub.test_email': 'Test email',
            'sub.settings': 'Settings',
            'sub.users': 'Users',
            'sub.api_keys': 'API keys',
            'sub.tracking': 'Tracking',
            'sub.audit': 'Audit log',
            'logout': 'Logout',

            // Common
            'actions': 'Actions',
            'save': 'Save',
            'cancel': 'Cancel',
            'delete': 'Delete',
            'edit': 'Edit',
            'view': 'View',
            'create': 'Create',
            'search': 'Search',
            'filter': 'Filter',
            'loading': 'Loading...',
            'confirm_delete': 'Are you sure you want to delete this?',

            // Templates
            'templates': 'Templates',
            'template_html': 'HTML Content',
            'template_text': 'Text Content',
            'template_variables': 'Variables',
            'template_preview': 'Preview',
            'template_deploy': 'Deploy',
            'current_version': 'Current Version',

            // Recipients
            'recipients': 'Recipients',
            'import_csv': 'Import CSV',
            'email': 'Email',
            'name': 'Name',
            'status': 'Status',

            // Campaigns
            'campaigns': 'Campaigns',

            // Jobs
            'pause': 'Pause',
            'resume': 'Resume',
            'theme_toggle': 'Toggle theme',

            // Servers
            'server_sandbox': 'Sandbox',
            'online': 'Online',
            'offline': 'Offline',

            // Settings
            'settings': 'Settings',
            'display_preferences': 'Display Preferences',
            'system': 'System',
            'servers': 'Servers',
            'domains': 'Domains',
            'dkim_keys': 'DKIM Keys',
            'users': 'Users',
            'users_desc': 'Accounts and roles for this panel',
            'audit_log': 'Audit log',
            'audit_log_desc': 'Who changed what, and when',
            'api_keys': 'API keys',
            'api_keys_desc': 'Keys for sending through the HTTP API',
            'send_test_email': 'Send Test Email',
            'mta_server': 'MTA Server',
            'from': 'From',
            'to': 'To',
            'subject': 'Subject',
            'body': 'Body',
            'send_as_html': 'Send as HTML',
            'timezone': 'Timezone',
            'timezone_help': 'All dates will be displayed in selected timezone',

            // Monitoring
            'monitoring.title': 'Monitoring',
            'monitoring.refresh': 'Refresh',
            'monitoring.period.24h': 'Last 24 hours',
            'monitoring.period.7d': 'Last 7 days',
            'monitoring.period.30d': 'Last 30 days',
            'monitoring.domain.all': 'All Domains',
            'monitoring.stats.sent': 'Sent',
            'monitoring.stats.failed': 'Failed',
            'monitoring.stats.queue': 'In Queue',
            'monitoring.stats.dlq': 'DLQ',
            'monitoring.chart.activity': 'Send Activity',
            'monitoring.chart.domains': 'By Domain',
            'monitoring.servers.title': 'Server Status',
            'monitoring.servers.name': 'Server',
            'monitoring.servers.env': 'Environment',
            'monitoring.servers.status': 'Status',
            'monitoring.servers.queue': 'Queue',
            'monitoring.servers.dlq': 'DLQ',
            'monitoring.servers.empty': 'No servers configured',

            // Queue overview
            'queue.title': 'Queue',
            'queue.total': 'Total',
            'queue.queue_dlq_hint': '(queue / DLQ)',
            'queue.tab_queue': 'Queue',
            'queue.tab_dlq': 'DLQ',
            'queue.messages_in_queue': 'messages in queue',
            'queue.messages_in_dlq': 'messages in DLQ',
            'queue.empty': 'Queue is empty',
            'queue.empty_hint': 'Messages waiting to be sent will appear here',
            'queue.dlq_empty': 'DLQ is empty',
            'queue.dlq_empty_hint': 'Failed messages will appear here'
        },
        ru: {
            // Navigation
            'nav.overview': 'Обзор',
            'nav.campaigns': 'Рассылки',
            'nav.templates': 'Шаблоны',
            'nav.audience': 'Аудитория',
            'nav.sends': 'Отправки по API',
            'nav.infra': 'Инфраструктура',
            'nav.admin': 'Администрирование',
            'nav.group_email': 'Почта',
            'nav.group_system': 'Система',
            'nav.collapse': 'Свернуть меню',
            'nav.expand': 'Развернуть меню',
            'nav.section_pages': 'Страницы раздела',
            'nav.queue_badge': 'Писем ждёт отправки в очереди',
            'sub.campaigns': 'Рассылки',
            'sub.runs': 'Запуски',
            'sub.templates': 'Шаблоны',
            'sub.blocks': 'Блоки',
            'sub.media': 'Медиа',
            'sub.variables': 'Переменные',
            'sub.servers': 'Серверы',
            'sub.queue': 'Очередь',
            'sub.domains': 'Домены',
            'sub.dkim': 'DKIM',
            'sub.monitoring': 'Мониторинг',
            'sub.test_email': 'Тестовое письмо',
            'sub.settings': 'Настройки',
            'sub.users': 'Пользователи',
            'sub.api_keys': 'API-ключи',
            'sub.tracking': 'Трекинг',
            'sub.audit': 'Журнал аудита',
            'logout': 'Выход',

            // Common
            'actions': 'Действия',
            'save': 'Сохранить',
            'cancel': 'Отмена',
            'delete': 'Удалить',
            'edit': 'Редактировать',
            'view': 'Просмотр',
            'create': 'Создать',
            'search': 'Поиск',
            'filter': 'Фильтр',
            'loading': 'Загрузка...',
            'confirm_delete': 'Вы уверены, что хотите удалить?',

            // Templates
            'templates': 'Шаблоны',
            'template_html': 'HTML содержимое',
            'template_text': 'Текстовое содержимое',
            'template_variables': 'Переменные',
            'template_preview': 'Предпросмотр',
            'template_deploy': 'Деплой',
            'current_version': 'Текущая версия',

            // Recipients
            'recipients': 'Получатели',
            'import_csv': 'Импорт CSV',
            'email': 'Email',
            'name': 'Имя',
            'status': 'Статус',

            // Campaigns
            'campaigns': 'Рассылки',

            // Jobs
            'pause': 'Пауза',
            'resume': 'Продолжить',
            'theme_toggle': 'Сменить тему',

            // Servers
            'server_sandbox': 'Песочница',
            'online': 'Онлайн',
            'offline': 'Офлайн',

            // Settings
            'settings': 'Настройки',
            'display_preferences': 'Настройки отображения',
            'system': 'Система',
            'servers': 'Серверы',
            'domains': 'Домены',
            'dkim_keys': 'Ключи DKIM',
            'users': 'Пользователи',
            'users_desc': 'Учётные записи и роли в этой панели',
            'audit_log': 'Журнал аудита',
            'audit_log_desc': 'Кто, что и когда изменил',
            'api_keys': 'API-ключи',
            'api_keys_desc': 'Ключи для отправки через HTTP API',
            'send_test_email': 'Отправить тестовое письмо',
            'mta_server': 'MTA Сервер',
            'from': 'От',
            'to': 'Кому',
            'subject': 'Тема',
            'body': 'Текст',
            'send_as_html': 'Отправить как HTML',
            'timezone': 'Часовой пояс',
            'timezone_help': 'Все даты будут отображаться в выбранном часовом поясе',

            // Monitoring
            'monitoring.title': 'Мониторинг',
            'monitoring.refresh': 'Обновить',
            'monitoring.period.24h': 'За 24 часа',
            'monitoring.period.7d': 'За 7 дней',
            'monitoring.period.30d': 'За 30 дней',
            'monitoring.domain.all': 'Все домены',
            'monitoring.stats.sent': 'Отправлено',
            'monitoring.stats.failed': 'Ошибки',
            'monitoring.stats.queue': 'В очереди',
            'monitoring.stats.dlq': 'Недоставленные (DLQ)',
            'monitoring.chart.activity': 'Активность отправок',
            'monitoring.chart.domains': 'По доменам',
            'monitoring.servers.title': 'Статус серверов',
            'monitoring.servers.name': 'Сервер',
            'monitoring.servers.env': 'Окружение',
            'monitoring.servers.status': 'Статус',
            'monitoring.servers.queue': 'Очередь',
            'monitoring.servers.dlq': 'Недоставленные (DLQ)',
            'monitoring.servers.empty': 'Серверы не настроены',

            // Queue overview
            'queue.title': 'Очередь',
            'queue.total': 'Всего',
            'queue.queue_dlq_hint': '(очередь / недоставленные)',
            'queue.tab_queue': 'Очередь',
            'queue.tab_dlq': 'Недоставленные (DLQ)',
            'queue.messages_in_queue': 'сообщений в очереди',
            'queue.messages_in_dlq': 'недоставленных сообщений',
            'queue.empty': 'Очередь пуста',
            'queue.empty_hint': 'Сообщения, ожидающие отправки, появятся здесь',
            'queue.dlq_empty': 'Недоставленных сообщений нет',
            'queue.dlq_empty_hint': 'Неотправленные сообщения появятся здесь'
        }
    };

    var currentLang = localStorage.getItem('lang') || 'en';

    // The server picks the language of generated names (e.g. "(copy)") from this cookie
    function rememberLang(lang) {
        document.cookie = 'lang=' + lang + '; path=/; max-age=31536000; SameSite=Lax';
    }
    rememberLang(currentLang);

    // Merge section dictionaries ({en: {...}, ru: {...}}) loaded from /static/js/i18n/*.js
    function register(dict) {
        for (var lang in dict) {
            if (!translations[lang]) translations[lang] = {};
            for (var k in dict[lang]) translations[lang][k] = dict[lang][k];
        }
    }

    function lookup(key) {
        var v = translations[currentLang][key];
        if (v === undefined) v = translations['en'][key];
        return v;
    }

    function t(key, params) {
        var text = lookup(key);
        if (text === undefined) text = key;
        if (params) {
            for (var p in params) text = fillParam(text, p, params[p]);
        }
        return text;
    }

    // Replaces the first {name} in text with value as is: String.replace
    // would read "$&" or "$'" in a campaign name or domain as a pattern
    function fillParam(text, name, value) {
        var marker = '{' + name + '}';
        var i = text.indexOf(marker);
        if (i < 0) return text;
        return text.slice(0, i) + String(value) + text.slice(i + marker.length);
    }

    function setLang(lang) {
        if (translations[lang]) {
            currentLang = lang;
            localStorage.setItem('lang', lang);
            rememberLang(lang);
            document.documentElement.lang = lang;
            applyTranslations();
            updateLangButtons();
        }
    }

    function getLang() {
        return currentLang;
    }

    function applyTranslations() {
        // Unknown keys leave the server-rendered text in place;
        // data-i18n-arg-<name> attributes fill {name} placeholders
        document.querySelectorAll('[data-i18n]').forEach(function(el) {
            var key = el.getAttribute('data-i18n');
            var v = lookup(key);
            if (v === undefined) return;
            for (var i = 0; i < el.attributes.length; i++) {
                var a = el.attributes[i];
                if (a.name.indexOf('data-i18n-arg-') === 0) v = fillParam(v, a.name.slice(14), a.value);
            }
            el.textContent = v;
        });
        document.querySelectorAll('[data-i18n-placeholder]').forEach(function(el) {
            var key = el.getAttribute('data-i18n-placeholder');
            el.placeholder = t(key);
        });
        document.querySelectorAll('[data-i18n-title]').forEach(function(el) {
            var key = el.getAttribute('data-i18n-title');
            el.title = t(key);
            if (el.hasAttribute('aria-label')) el.setAttribute('aria-label', t(key));
        });
        document.querySelectorAll('[data-i18n-aria]').forEach(function(el) {
            el.setAttribute('aria-label', t(el.getAttribute('data-i18n-aria')));
        });
        // data-label feeds CSS ::before labels in stacked (mobile) tables
        document.querySelectorAll('[data-i18n-label]').forEach(function(el) {
            var key = el.getAttribute('data-i18n-label');
            el.setAttribute('data-label', t(key));
        });
    }

    function updateLangButtons() {
        document.querySelectorAll('.lang-btn').forEach(function(btn) {
            var lang = btn.getAttribute('data-lang');
            btn.classList.toggle('active', lang === currentLang);
        });
    }

    function init() {
        applyTranslations();
        updateLangButtons();
        // Set by the inline script in <head> to hide the untranslated page
        document.documentElement.removeAttribute('data-i18n-pending');

        document.querySelectorAll('.lang-btn').forEach(function(btn) {
            btn.addEventListener('click', function() {
                setLang(this.getAttribute('data-lang'));
            });
        });
    }

    return {
        t: t,
        apply: applyTranslations,
        register: register,
        setLang: setLang,
        getLang: getLang,
        init: init
    };
})();

document.addEventListener('DOMContentLoaded', function() {
    I18n.init();
});
