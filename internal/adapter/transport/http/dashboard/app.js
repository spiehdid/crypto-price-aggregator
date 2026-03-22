function dashboard() {
    return {
        currentPage: 'overview',
        darkMode: localStorage.getItem('darkMode') !== 'false',
        sidebarOpen: true,

        // Data
        stats: {},
        providers: [],
        prices: {},
        alerts: [],
        subscriptions: [],
        systemReady: false,

        // Chart
        chartInstance: null,
        chartCoin: 'bitcoin',
        chartInterval: '1h',
        chartFrom: '',
        chartTo: '',

        // Forms
        alertForm: { coin_id: '', condition: 'above', threshold: '', webhook_url: '' },
        subForm: { coin_id: '', currency: 'usd', interval: '60s' },
        convertForm: { from: 'bitcoin', to: 'ethereum', amount: '1' },
        convertResult: null,

        // Search
        priceSearch: '',
        providerSearch: '',

        // WebSocket
        ws: null,

        init() {
            this.applyTheme();
            this.setDefaultDates();
            this.fetchAll();
            this.connectWebSocket();

            setInterval(() => this.fetchStats(), 5000);
            setInterval(() => this.fetchProviders(), 5000);
            setInterval(() => this.fetchAlerts(), 10000);
            setInterval(() => this.fetchSubscriptions(), 10000);
            setInterval(() => this.checkHealth(), 5000);
        },

        // Theme
        toggleTheme() {
            this.darkMode = !this.darkMode;
            localStorage.setItem('darkMode', this.darkMode);
            this.applyTheme();
        },
        applyTheme() {
            document.documentElement.classList.toggle('dark', this.darkMode);
        },

        setDefaultDates() {
            const now = new Date();
            const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000);
            this.chartTo = now.toISOString().slice(0, 16);
            this.chartFrom = yesterday.toISOString().slice(0, 16);
        },

        fetchAll() {
            this.fetchStats();
            this.fetchProviders();
            this.fetchAlerts();
            this.fetchSubscriptions();
            this.checkHealth();
        },

        async fetchStats() {
            try {
                const r = await fetch('/api/v1/admin/stats');
                if (r.ok) this.stats = await r.json();
            } catch (e) { /* ignore */ }
        },

        async fetchProviders() {
            try {
                const r = await fetch('/api/v1/admin/providers');
                if (r.ok) {
                    const data = await r.json();
                    this.providers = data.providers || [];
                }
            } catch (e) { /* ignore */ }
        },

        async fetchAlerts() {
            try {
                const r = await fetch('/api/v1/alerts');
                if (r.ok) {
                    const data = await r.json();
                    this.alerts = data.alerts || [];
                }
            } catch (e) { /* ignore */ }
        },

        async fetchSubscriptions() {
            try {
                const r = await fetch('/api/v1/admin/subscriptions');
                if (r.ok) {
                    const data = await r.json();
                    this.subscriptions = data.subscriptions || [];
                }
            } catch (e) { /* ignore */ }
        },

        async checkHealth() {
            try {
                const r = await fetch('/readyz');
                if (r.ok) {
                    const data = await r.json();
                    this.systemReady = data.status === 'ready';
                } else {
                    this.systemReady = false;
                }
            } catch (e) { this.systemReady = false; }
        },

        // WebSocket
        connectWebSocket() {
            const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
            try {
                this.ws = new WebSocket(`${proto}//${location.host}/ws/prices`);
                this.ws.onopen = () => {
                    if (this.ws.readyState === WebSocket.OPEN) {
                        this.ws.send(JSON.stringify({
                            action: 'subscribe',
                            coins: ['bitcoin', 'ethereum', 'solana', 'cardano', 'ripple', 'dogecoin', 'polkadot', 'chainlink', 'avalanche', 'litecoin']
                        }));
                    }
                };
                this.ws.onmessage = (e) => {
                    try {
                        const price = JSON.parse(e.data);
                        this.prices[price.coin] = { ...price, updated: Date.now() };
                    } catch (err) { /* ignore */ }
                };
                this.ws.onclose = () => {
                    setTimeout(() => this.connectWebSocket(), 3000);
                };
            } catch (e) { /* ignore */ }
        },

        // Price helpers
        get sortedPrices() {
            return Object.values(this.prices)
                .filter(p => !this.priceSearch || p.coin.includes(this.priceSearch.toLowerCase()))
                .sort((a, b) => a.coin.localeCompare(b.coin));
        },

        get filteredProviders() {
            return this.providers
                .filter(p => !this.providerSearch || p.Name.toLowerCase().includes(this.providerSearch.toLowerCase()))
                .sort((a, b) => a.Name.localeCompare(b.Name));
        },

        get topPrices() {
            return Object.values(this.prices)
                .sort((a, b) => parseFloat(b.price || 0) - parseFloat(a.price || 0))
                .slice(0, 5);
        },

        get topProviders() {
            return this.providers
                .sort((a, b) => a.Name.localeCompare(b.Name))
                .slice(0, 6);
        },

        // Actions
        async createAlert() {
            try {
                const body = {
                    coin_id: this.alertForm.coin_id,
                    condition: this.alertForm.condition,
                    threshold: this.alertForm.threshold,
                    webhook_url: this.alertForm.webhook_url
                };
                const r = await fetch('/api/v1/alerts', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body)
                });
                if (r.ok) {
                    this.alertForm = { coin_id: '', condition: 'above', threshold: '', webhook_url: '' };
                    this.fetchAlerts();
                }
            } catch (e) { /* ignore */ }
        },

        async deleteAlert(id) {
            try {
                await fetch(`/api/v1/alerts/${id}`, { method: 'DELETE' });
                this.fetchAlerts();
            } catch (e) { /* ignore */ }
        },

        async addSubscription() {
            try {
                const r = await fetch('/api/v1/admin/subscriptions', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(this.subForm)
                });
                if (r.ok) {
                    this.subForm = { coin_id: '', currency: 'usd', interval: '60s' };
                    this.fetchSubscriptions();
                }
            } catch (e) { /* ignore */ }
        },

        async removeSubscription(coinId) {
            try {
                await fetch(`/api/v1/admin/subscriptions/${coinId}`, { method: 'DELETE' });
                this.fetchSubscriptions();
            } catch (e) { /* ignore */ }
        },

        async convert() {
            try {
                const r = await fetch(`/api/v1/convert?from=${this.convertForm.from}&to=${this.convertForm.to}&amount=${this.convertForm.amount}`);
                if (r.ok) this.convertResult = await r.json();
            } catch (e) { /* ignore */ }
        },

        // Chart
        async loadChart() {
            try {
                const from = new Date(this.chartFrom).toISOString();
                const to = new Date(this.chartTo).toISOString();
                const r = await fetch(`/api/v1/ohlc/${this.chartCoin}?currency=usd&from=${from}&to=${to}&interval=${this.chartInterval}`);
                if (!r.ok) return;
                const data = await r.json();
                this.renderChart(data.candles || []);
            } catch (e) { /* ignore */ }
        },

        renderChart(candles) {
            const ctx = document.getElementById('priceChart');
            if (!ctx) return;

            if (this.chartInstance) {
                this.chartInstance.destroy();
            }

            const labels = candles.map(c => new Date(c.Time).toLocaleString());
            const closes = candles.map(c => parseFloat(c.Close));

            this.chartInstance = new Chart(ctx, {
                type: 'line',
                data: {
                    labels: labels,
                    datasets: [{
                        label: `${this.chartCoin} (${this.chartInterval})`,
                        data: closes,
                        borderColor: '#38bdf8',
                        backgroundColor: 'rgba(56, 189, 248, 0.1)',
                        fill: true,
                        tension: 0.3,
                        pointRadius: 0
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    plugins: {
                        legend: {
                            labels: { color: this.darkMode ? '#e2e8f0' : '#1e293b' }
                        }
                    },
                    scales: {
                        x: {
                            ticks: { color: this.darkMode ? '#94a3b8' : '#64748b', maxTicksLimit: 10 },
                            grid: { color: this.darkMode ? '#334155' : '#e2e8f0' }
                        },
                        y: {
                            ticks: { color: this.darkMode ? '#94a3b8' : '#64748b' },
                            grid: { color: this.darkMode ? '#334155' : '#e2e8f0' }
                        }
                    }
                }
            });
        },

        // Helpers
        formatPrice(val) {
            const n = parseFloat(val);
            if (isNaN(n)) return '-';
            if (n >= 1) return n.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
            return n.toLocaleString('en-US', { minimumFractionDigits: 4, maximumFractionDigits: 8 });
        },

        formatMs(ns) {
            if (!ns) return '-';
            const ms = ns / 1000000;
            return ms.toFixed(0) + 'ms';
        },

        formatUptime(seconds) {
            if (!seconds) return '-';
            const h = Math.floor(seconds / 3600);
            const m = Math.floor((seconds % 3600) / 60);
            if (h > 0) return `${h}h ${m}m`;
            return `${m}m`;
        },

        coinDisplayName(coin) {
            if (!coin) return '';
            return coin.charAt(0).toUpperCase() + coin.slice(1);
        }
    };
}
