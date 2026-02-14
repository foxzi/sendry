(function() {
    'use strict';

    var MonitoringCharts = {
        timeSeriesChart: null,
        domainChart: null,
        currentPeriod: '30d',
        currentDomain: '',

        init: function() {
            this.initEventListeners();
            this.loadData();
            this.loadServers();
        },

        initEventListeners: function() {
            var self = this;

            var periodSelect = document.getElementById('period-selector');
            if (periodSelect) {
                periodSelect.addEventListener('change', function() {
                    self.currentPeriod = this.value;
                    self.loadData();
                });
            }

            var domainSelect = document.getElementById('domain-filter');
            if (domainSelect) {
                domainSelect.addEventListener('change', function() {
                    self.currentDomain = this.value;
                    self.loadData();
                });
            }

            var refreshBtn = document.getElementById('refresh-btn');
            if (refreshBtn) {
                refreshBtn.addEventListener('click', function() {
                    self.loadData();
                    self.loadServers();
                });
            }
        },

        loadData: function() {
            var self = this;
            var url = '/monitoring/api/stats?period=' + this.currentPeriod;
            if (this.currentDomain) {
                url += '&domain=' + encodeURIComponent(this.currentDomain);
            }

            fetch(url)
                .then(function(response) { return response.json(); })
                .then(function(data) {
                    self.updateStats(data);
                    self.renderTimeSeriesChart(data.time_series || []);
                    self.renderDomainChart(data.domain_stats || []);
                })
                .catch(function(error) {
                    console.error('Failed to load monitoring data:', error);
                });
        },

        loadServers: function() {
            var self = this;
            fetch('/monitoring/api/servers')
                .then(function(response) { return response.json(); })
                .then(function(servers) {
                    self.updateServerStats(servers);
                })
                .catch(function(error) {
                    console.error('Failed to load server data:', error);
                });
        },

        updateStats: function(data) {
            var sentEl = document.getElementById('total-sent');
            var failedEl = document.getElementById('total-failed');

            if (sentEl) sentEl.textContent = data.total_sent || 0;
            if (failedEl) failedEl.textContent = data.total_failed || 0;
        },

        updateServerStats: function(servers) {
            var totalQueue = 0;
            var totalDlq = 0;

            servers.forEach(function(server) {
                totalQueue += server.queue_size || 0;
                totalDlq += server.dlq_size || 0;
            });

            var queueEl = document.getElementById('total-queue');
            var dlqEl = document.getElementById('total-dlq');

            if (queueEl) queueEl.textContent = totalQueue;
            if (dlqEl) dlqEl.textContent = totalDlq;

            this.updateServerTable(servers);
        },

        updateServerTable: function(servers) {
            var tbody = document.getElementById('servers-tbody');
            if (!tbody) return;

            tbody.innerHTML = '';

            servers.forEach(function(server) {
                var tr = document.createElement('tr');
                tr.innerHTML =
                    '<td><a href="/servers/' + server.name + '">' + server.name + '</a></td>' +
                    '<td><span class="badge badge-' + server.env + '">' + server.env + '</span></td>' +
                    '<td>' + (server.online
                        ? '<span class="badge badge-running">Online</span>'
                        : '<span class="badge badge-failed">Offline</span>') + '</td>' +
                    '<td>' + (server.queue_size || 0) + '</td>' +
                    '<td>' + (server.dlq_size || 0) + '</td>';
                tbody.appendChild(tr);
            });
        },

        renderTimeSeriesChart: function(data) {
            var ctx = document.getElementById('timeseries-chart');
            if (!ctx) return;

            var labels = data.map(function(point) {
                var date = new Date(point.timestamp);
                if (this.currentPeriod === '24h') {
                    return date.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
                }
                return date.toLocaleDateString([], {month: 'short', day: 'numeric'});
            }, this);

            var sentData = data.map(function(point) { return point.sent; });
            var failedData = data.map(function(point) { return point.failed; });
            var pendingData = data.map(function(point) { return point.pending; });

            var isDark = document.documentElement.getAttribute('data-theme') === 'dark';
            var gridColor = isDark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)';
            var textColor = isDark ? '#ccc' : '#666';

            if (this.timeSeriesChart) {
                this.timeSeriesChart.destroy();
            }

            this.timeSeriesChart = new Chart(ctx, {
                type: 'line',
                data: {
                    labels: labels,
                    datasets: [
                        {
                            label: 'Sent',
                            data: sentData,
                            borderColor: '#10b981',
                            backgroundColor: 'rgba(16, 185, 129, 0.1)',
                            fill: true,
                            tension: 0.3
                        },
                        {
                            label: 'Failed',
                            data: failedData,
                            borderColor: '#ef4444',
                            backgroundColor: 'rgba(239, 68, 68, 0.1)',
                            fill: true,
                            tension: 0.3
                        },
                        {
                            label: 'Pending',
                            data: pendingData,
                            borderColor: '#f59e0b',
                            backgroundColor: 'rgba(245, 158, 11, 0.1)',
                            fill: true,
                            tension: 0.3
                        }
                    ]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    interaction: {
                        intersect: false,
                        mode: 'index'
                    },
                    plugins: {
                        legend: {
                            position: 'top',
                            labels: { color: textColor }
                        }
                    },
                    scales: {
                        x: {
                            grid: { color: gridColor },
                            ticks: { color: textColor }
                        },
                        y: {
                            beginAtZero: true,
                            grid: { color: gridColor },
                            ticks: { color: textColor }
                        }
                    }
                }
            });
        },

        renderDomainChart: function(data) {
            var ctx = document.getElementById('domain-chart');
            if (!ctx) return;

            var labels = data.map(function(d) { return d.domain; });
            var sentData = data.map(function(d) { return d.sent; });
            var failedData = data.map(function(d) { return d.failed; });

            var isDark = document.documentElement.getAttribute('data-theme') === 'dark';
            var gridColor = isDark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)';
            var textColor = isDark ? '#ccc' : '#666';

            if (this.domainChart) {
                this.domainChart.destroy();
            }

            this.domainChart = new Chart(ctx, {
                type: 'bar',
                data: {
                    labels: labels,
                    datasets: [
                        {
                            label: 'Sent',
                            data: sentData,
                            backgroundColor: '#10b981'
                        },
                        {
                            label: 'Failed',
                            data: failedData,
                            backgroundColor: '#ef4444'
                        }
                    ]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    indexAxis: 'y',
                    plugins: {
                        legend: {
                            position: 'top',
                            labels: { color: textColor }
                        }
                    },
                    scales: {
                        x: {
                            stacked: true,
                            beginAtZero: true,
                            grid: { color: gridColor },
                            ticks: { color: textColor }
                        },
                        y: {
                            stacked: true,
                            grid: { color: gridColor },
                            ticks: { color: textColor }
                        }
                    }
                }
            });
        }
    };

    window.MonitoringCharts = MonitoringCharts;

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function() {
            MonitoringCharts.init();
        });
    } else {
        MonitoringCharts.init();
    }
})();
