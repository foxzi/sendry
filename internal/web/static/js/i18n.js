// Internationalization support
var I18n = (function() {
    var translations = {
        en: {
            // Navigation
            'nav.templates': 'Templates',
            'nav.recipients': 'Recipients',
            'nav.campaigns': 'Campaigns',
            'nav.jobs': 'Jobs',
            'nav.sends': 'Sends',
            'nav.servers': 'Servers',
            'nav.monitoring': 'Monitoring',
            'nav.settings': 'Settings',
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
            'users_desc': 'Manage user accounts and permissions',
            'audit_log': 'Audit Log',
            'audit_log_desc': 'View activity history and changes',
            'api_keys': 'API Keys',
            'api_keys_desc': 'Manage API keys for external integrations',
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
            'monitoring.servers.empty': 'No servers configured'
        },
        ru: {
            // Navigation
            'nav.templates': 'Шаблоны',
            'nav.recipients': 'Получатели',
            'nav.campaigns': 'Кампании',
            'nav.jobs': 'Рассылки',
            'nav.sends': 'Отправки',
            'nav.servers': 'Серверы',
            'nav.monitoring': 'Мониторинг',
            'nav.settings': 'Настройки',
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
            'campaigns_count': 'Кампании',
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
            'campaigns': 'Кампании',
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
            'users_desc': 'Управление учётными записями',
            'audit_log': 'Журнал действий',
            'audit_log_desc': 'Просмотр истории изменений',
            'api_keys': 'API ключи',
            'api_keys_desc': 'Управление ключами для внешних интеграций',
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
            'monitoring.servers.empty': 'Серверы не настроены'
        }
    };

    var currentLang = localStorage.getItem('lang') || 'en';

    function t(key, params) {
        var text = translations[currentLang][key] || translations['en'][key] || key;
        if (params) {
            for (var p in params) {
                text = text.replace('{' + p + '}', params[p]);
            }
        }
        return text;
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
        document.querySelectorAll('[data-i18n]').forEach(function(el) {
            var key = el.getAttribute('data-i18n');
            el.textContent = t(key);
        });
        document.querySelectorAll('[data-i18n-placeholder]').forEach(function(el) {
            var key = el.getAttribute('data-i18n-placeholder');
            el.placeholder = t(key);
        });
        document.querySelectorAll('[data-i18n-title]').forEach(function(el) {
            var key = el.getAttribute('data-i18n-title');
            el.title = t(key);
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
        setLang: setLang,
        getLang: getLang,
        init: init
    };
})();

document.addEventListener('DOMContentLoaded', function() {
    I18n.init();
});
