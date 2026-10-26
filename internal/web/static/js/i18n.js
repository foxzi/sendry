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
            'nav.menu': 'Menu',
            'nav.collapse': 'Collapse menu',
            'nav.expand': 'Expand menu',
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
            'no_data': 'No data',
            'confirm_delete': 'Are you sure you want to delete this?',

            // Dashboard
            'dashboard': 'Dashboard',
            'templates_count': 'Templates',
            'campaigns_count': 'Campaigns',
            'recipients_count': 'Recipients',
            'active_jobs': 'Active Jobs',
            'servers_status': 'Servers Status',
            'recent_jobs': 'Recent Jobs',

            // Templates
            'templates': 'Templates',
            'new_template': 'New Template',
            'template_name': 'Name',
            'template_subject': 'Subject',
            'template_description': 'Description',
            'template_html': 'HTML Content',
            'template_text': 'Text Content',
            'template_variables': 'Variables',
            'template_preview': 'Preview',
            'template_deploy': 'Deploy',
            'template_versions': 'Versions',
            'current_version': 'Current Version',

            // Recipients
            'recipients': 'Recipients',
            'recipient_lists': 'Recipient Lists',
            'new_list': 'New List',
            'list_name': 'List Name',
            'import_csv': 'Import CSV',
            'import_json': 'Import JSON',
            'total_recipients': 'Total',
            'active_recipients': 'Active',
            'email': 'Email',
            'name': 'Name',
            'status': 'Status',
            'status_active': 'Active',
            'status_unsubscribed': 'Unsubscribed',
            'status_bounced': 'Bounced',

            // Campaigns
            'campaigns': 'Campaigns',
            'new_campaign': 'New Campaign',
            'campaign_name': 'Campaign Name',
            'from_email': 'From Email',
            'from_name': 'From Name',
            'reply_to': 'Reply To',
            'campaign_variables': 'Variables',
            'campaign_variants': 'Variants',
            'send_campaign': 'Send',

            // Jobs
            'jobs': 'Jobs',
            'job_status': 'Status',
            'job_progress': 'Progress',
            'job_created': 'Created',
            'job_started': 'Started',
            'job_completed': 'Completed',
            'pause': 'Pause',
            'resume': 'Resume',
            'theme_toggle': 'Toggle theme',
            'cancel_job': 'Cancel',
            'retry_failed': 'Retry Failed',
            'status_draft': 'Draft',
            'status_scheduled': 'Scheduled',
            'status_running': 'Running',
            'status_paused': 'Paused',
            'status_completed': 'Completed',
            'status_failed': 'Failed',
            'status_cancelled': 'Cancelled',

            // Servers
            'servers': 'Servers',
            'server_name': 'Server',
            'server_status': 'Status',
            'server_queue': 'Queue',
            'server_dlq': 'DLQ',
            'server_domains': 'Domains',
            'server_sandbox': 'Sandbox',
            'online': 'Online',
            'offline': 'Offline',

            // Settings
            'settings': 'Settings',
            'display_preferences': 'Display Preferences',
            'infrastructure': 'Infrastructure',
            'system': 'System',
            'servers': 'Servers',
            'servers_desc': 'Manage Sendry MTA servers, queues and sandbox',
            'domains': 'Domains',
            'domains_desc': 'Configure sending domains with DKIM, rate limits and modes',
            'dkim_keys': 'DKIM Keys',
            'dkim_keys_desc': 'Generate and deploy DKIM signing keys to servers',
            'global_variables': 'Global Variables',
            'global_variables_desc': 'Manage template variables available across all campaigns',
            'users': 'Users',
            'users_desc': 'Accounts and roles for this panel',
            'audit_log': 'Audit log',
            'audit_log_desc': 'Who changed what, and when',
            'api_keys': 'API keys',
            'api_keys_desc': 'Keys for sending through the HTTP API',
            'send_test_email': 'Send Test Email',
            'send_test_email_desc': 'Send a test email through any MTA server',
            'mta_server': 'MTA Server',
            'from': 'From',
            'to': 'To',
            'subject': 'Subject',
            'body': 'Body',
            'send_as_html': 'Send as HTML',
            'back_to_settings': 'Back to Settings',
            'timezone': 'Timezone',
            'timezone_help': 'All dates will be displayed in selected timezone',
            'language': 'Language',

            // Time
            'just_now': 'Just now',
            'minutes_ago': '{n} min ago',
            'hours_ago': '{n} hours ago',
            'days_ago': '{n} days ago',

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
            'nav.menu': 'Меню',
            'nav.collapse': 'Свернуть меню',
            'nav.expand': 'Развернуть меню',
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
            'no_data': 'Нет данных',
            'confirm_delete': 'Вы уверены, что хотите удалить?',

            // Dashboard
            'dashboard': 'Панель управления',
            'templates_count': 'Шаблоны',
            'campaigns_count': 'Рассылки',
            'recipients_count': 'Получатели',
            'active_jobs': 'Активные рассылки',
            'servers_status': 'Статус серверов',
            'recent_jobs': 'Недавние рассылки',

            // Templates
            'templates': 'Шаблоны',
            'new_template': 'Новый шаблон',
            'template_name': 'Название',
            'template_subject': 'Тема письма',
            'template_description': 'Описание',
            'template_html': 'HTML содержимое',
            'template_text': 'Текстовое содержимое',
            'template_variables': 'Переменные',
            'template_preview': 'Предпросмотр',
            'template_deploy': 'Деплой',
            'template_versions': 'Версии',
            'current_version': 'Текущая версия',

            // Recipients
            'recipients': 'Получатели',
            'recipient_lists': 'Списки получателей',
            'new_list': 'Новый список',
            'list_name': 'Название списка',
            'import_csv': 'Импорт CSV',
            'import_json': 'Импорт JSON',
            'total_recipients': 'Всего',
            'active_recipients': 'Активных',
            'email': 'Email',
            'name': 'Имя',
            'status': 'Статус',
            'status_active': 'Активен',
            'status_unsubscribed': 'Отписан',
            'status_bounced': 'Отклонён',

            // Campaigns
            'campaigns': 'Рассылки',
            'new_campaign': 'Новая кампания',
            'campaign_name': 'Название кампании',
            'from_email': 'Email отправителя',
            'from_name': 'Имя отправителя',
            'reply_to': 'Адрес для ответа',
            'campaign_variables': 'Переменные',
            'campaign_variants': 'Варианты',
            'send_campaign': 'Отправить',

            // Jobs
            'jobs': 'Рассылки',
            'job_status': 'Статус',
            'job_progress': 'Прогресс',
            'job_created': 'Создана',
            'job_started': 'Запущена',
            'job_completed': 'Завершена',
            'pause': 'Пауза',
            'resume': 'Продолжить',
            'theme_toggle': 'Сменить тему',
            'cancel_job': 'Отменить',
            'retry_failed': 'Повторить неудачные',
            'status_draft': 'Черновик',
            'status_scheduled': 'Запланирована',
            'status_running': 'Выполняется',
            'status_paused': 'Приостановлена',
            'status_completed': 'Завершена',
            'status_failed': 'Ошибка',
            'status_cancelled': 'Отменена',

            // Servers
            'servers': 'Серверы',
            'server_name': 'Сервер',
            'server_status': 'Статус',
            'server_queue': 'Очередь',
            'server_dlq': 'DLQ',
            'server_domains': 'Домены',
            'server_sandbox': 'Песочница',
            'online': 'Онлайн',
            'offline': 'Офлайн',

            // Settings
            'settings': 'Настройки',
            'display_preferences': 'Настройки отображения',
            'infrastructure': 'Инфраструктура',
            'system': 'Система',
            'servers': 'Серверы',
            'servers_desc': 'Управление серверами Sendry MTA, очередями и песочницей',
            'domains': 'Домены',
            'domains_desc': 'Настройка доменов отправки: DKIM, лимиты, режимы',
            'dkim_keys': 'Ключи DKIM',
            'dkim_keys_desc': 'Генерация и деплой ключей DKIM подписи на серверы',
            'global_variables': 'Глобальные переменные',
            'global_variables_desc': 'Управление переменными шаблонов для всех кампаний',
            'users': 'Пользователи',
            'users_desc': 'Учётные записи и роли в этой панели',
            'audit_log': 'Журнал аудита',
            'audit_log_desc': 'Кто, что и когда изменил',
            'api_keys': 'API-ключи',
            'api_keys_desc': 'Ключи для отправки через HTTP API',
            'send_test_email': 'Отправить тестовое письмо',
            'send_test_email_desc': 'Отправить тестовое письмо через любой MTA сервер',
            'mta_server': 'MTA Сервер',
            'from': 'От',
            'to': 'Кому',
            'subject': 'Тема',
            'body': 'Текст',
            'send_as_html': 'Отправить как HTML',
            'back_to_settings': 'Назад к настройкам',
            'timezone': 'Часовой пояс',
            'timezone_help': 'Все даты будут отображаться в выбранном часовом поясе',
            'language': 'Язык',

            // Time
            'just_now': 'Только что',
            'minutes_ago': '{n} мин. назад',
            'hours_ago': '{n} ч. назад',
            'days_ago': '{n} дн. назад',

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
            'monitoring.stats.dlq': 'DLQ',
            'monitoring.chart.activity': 'Активность отправок',
            'monitoring.chart.domains': 'По доменам',
            'monitoring.servers.title': 'Статус серверов',
            'monitoring.servers.name': 'Сервер',
            'monitoring.servers.env': 'Окружение',
            'monitoring.servers.status': 'Статус',
            'monitoring.servers.queue': 'Очередь',
            'monitoring.servers.dlq': 'DLQ',
            'monitoring.servers.empty': 'Серверы не настроены',

            // Queue overview
            'queue.title': 'Очередь',
            'queue.total': 'Всего',
            'queue.queue_dlq_hint': '(очередь / DLQ)',
            'queue.tab_queue': 'Очередь',
            'queue.tab_dlq': 'DLQ',
            'queue.messages_in_queue': 'сообщений в очереди',
            'queue.messages_in_dlq': 'сообщений в DLQ',
            'queue.empty': 'Очередь пуста',
            'queue.empty_hint': 'Сообщения, ожидающие отправки, появятся здесь',
            'queue.dlq_empty': 'DLQ пуст',
            'queue.dlq_empty_hint': 'Неотправленные сообщения появятся здесь'
        }
    };

    var currentLang = localStorage.getItem('lang') || 'en';

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
