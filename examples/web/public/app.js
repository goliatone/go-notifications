class NotificationCenter {
    constructor() {
        this.ws = null
        this.currentUser = null
        this.unreadCount = 0
        this.currentPage = 1
        this.unreadOnlyFilter = false
    }

    async init() {
        // Check if already logged in
        const user = await this.fetchCurrentUser()
        if (user) {
            this.currentUser = user
            this.showAppScreen()
            await this.loadInbox()
            await this.loadPreferences()
            this.connectWebSocket()
        } else {
            this.showLoginScreen()
        }

        this.bindEvents()
    }

    showLoginScreen() {
        document.getElementById('login-screen').style.display = 'block'
        document.getElementById('app-screen').style.display = 'none'
    }

    showAppScreen() {
        document.getElementById('login-screen').style.display = 'none'
        document.getElementById('app-screen').style.display = 'block'

        document.getElementById('username').textContent = this.currentUser.name
        document.getElementById('locale-badge').textContent = this.currentUser.locale

        console.log('Current user admin status:', this.currentUser.admin)
        if (this.currentUser.admin) {
            console.log('Showing admin panel and loading users')
            document.getElementById('admin-panel').style.display = 'block'
            this.loadUsers()
            this.loadAvailableChannels()
        }
    }

    bindEvents() {
        // Login buttons
        document.querySelectorAll('.user-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const email = e.currentTarget.dataset.email
                this.login(email)
            })
        })

        // Logout
        document.getElementById('logout-btn')?.addEventListener('click', () => this.logout())

        // Quick actions
        document.getElementById('test-notification-btn')?.addEventListener('click', () => this.sendTestNotification())
        document.getElementById('mark-all-read-btn')?.addEventListener('click', () => this.markAllRead())
        document.getElementById('refresh-btn')?.addEventListener('click', () => this.loadInbox())
        document.getElementById('unread-only')?.addEventListener('change', (e) => {
            this.unreadOnlyFilter = e.target.checked
            this.currentPage = 1 // Reset to first page when filtering
            this.loadInbox()
        })

        // Admin actions
        document.getElementById('broadcast-btn')?.addEventListener('click', () => this.broadcastAlert())
        document.getElementById('view-stats-btn')?.addEventListener('click', () => this.viewStats())
        document.getElementById('send-to-user-btn')?.addEventListener('click', () => this.sendToUser())
    }

    async fetchCurrentUser() {
        try {
            const resp = await fetch('/api/user')
            if (resp.ok) {
                return await resp.json()
            }
        } catch (e) {
            console.error('Failed to fetch current user:', e)
        }
        return null
    }

    async login(email) {
        try {
            const resp = await fetch('/auth/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email })
            })

            if (resp.ok) {
                const data = await resp.json()
                this.currentUser = data.user
                this.showAppScreen()
                await this.loadInbox()
                await this.loadPreferences()
                this.connectWebSocket()
            } else {
                this.showToast('Login failed', 'error')
            }
        } catch (e) {
            this.showToast('Login error: ' + e.message, 'error')
        }
    }

    async logout() {
        try {
            await fetch('/auth/logout', { method: 'POST' })
            if (this.ws) {
                this.ws.close()
            }
            this.currentUser = null
            this.showLoginScreen()
        } catch (e) {
            console.error('Logout error:', e)
        }
    }

    connectWebSocket() {
        if (!this.currentUser) return

        const wsURL = `ws://${window.location.host}/ws?user_id=${this.currentUser.id}`
        this.ws = new WebSocket(wsURL)

        this.ws.onopen = () => {
            document.getElementById('ws-status').innerHTML = '🟢 Connected'
            this.showToast('Connected to real-time notifications', 'info')
        }

        this.ws.onclose = () => {
            document.getElementById('ws-status').innerHTML = '🔴 Disconnected'
        }

        this.ws.onerror = () => {
            document.getElementById('ws-status').innerHTML = '🔴 Error'
        }

        this.ws.onmessage = (event) => {
            try {
                const msg = JSON.parse(event.data)
                this.handleRealtimeEvent(msg)
            } catch (e) {
                console.error('Failed to parse WebSocket message:', e)
            }
        }
    }

    handleRealtimeEvent(msg) {
        console.log('Realtime event:', msg)

        switch(msg.event) {
            case 'inbox.new':
            case 'inbox.created':
                this.showToast('New notification received!', 'info')
                this.loadInbox()
                break
            case 'inbox.read':
            case 'inbox.dismissed':
            case 'inbox.updated':
                this.loadInbox()
                break
        }
    }

    async loadInbox() {
        try {
            const params = new URLSearchParams({
                page: this.currentPage,
                limit: 20,
                unread_only: this.unreadOnlyFilter
            })
            const resp = await fetch(`/api/inbox?${params}`)
            if (!resp.ok) throw new Error('Failed to load inbox')

            const data = await resp.json()
            this.renderInbox(data.items || [])
            this.unreadCount = data.unread_count || 0
            this.updateBadge()
        } catch (e) {
            console.error('Failed to load inbox:', e)
            this.showToast('Failed to load inbox', 'error')
        }
    }

    renderInbox(items) {
        const list = document.getElementById('inbox-list')

        if (items.length === 0) {
            list.innerHTML = `
                <div class="empty-state">
                    <h3>No notifications</h3>
                    <p>You're all caught up!</p>
                </div>
            `
            return
        }

        list.innerHTML = items.map(item => `
            <div class="inbox-item ${item.unread ? 'unread' : ''}" data-id="${item.id}">
                <div class="inbox-item-header">
                    <div class="inbox-item-title">${this.escapeHtml(item.title || 'Notification')}</div>
                    <div class="inbox-item-time">${this.formatTime(item.created_at)}</div>
                </div>
                <div class="inbox-item-body">${this.escapeHtml(item.body || 'No message content')}</div>
                <div class="inbox-item-actions">
                    ${item.unread ?
                        `<button class="btn btn-small btn-primary" onclick="app.markRead('${item.id}')">Mark Read</button>` :
                        `<button class="btn btn-small btn-secondary" onclick="app.markUnread('${item.id}')">Mark Unread</button>`
                    }
                    <button class="btn btn-small btn-danger" onclick="app.dismiss('${item.id}')">Dismiss</button>
                </div>
            </div>
        `).join('')
    }

    async markRead(id) {
        try {
            await fetch(`/api/inbox/${id}/read`, { method: 'POST' })
            this.loadInbox()
        } catch (e) {
            this.showToast('Failed to mark as read', 'error')
        }
    }

    async markUnread(id) {
        try {
            await fetch(`/api/inbox/${id}/unread`, { method: 'POST' })
            this.loadInbox()
        } catch (e) {
            this.showToast('Failed to mark as unread', 'error')
        }
    }

    async dismiss(id) {
        try {
            await fetch(`/api/inbox/${id}/dismiss`, { method: 'POST' })
            this.showToast('Notification dismissed', 'info')
            this.loadInbox()
        } catch (e) {
            this.showToast('Failed to dismiss', 'error')
        }
    }

    async markAllRead() {
        try {
            const resp = await fetch('/api/inbox/mark-all-read', { method: 'POST' })
            if (!resp.ok) throw new Error('Failed to mark all as read')
            const data = await resp.json()
            this.showToast(`Marked ${data.count} notifications as read`, 'info')
            this.loadInbox()
        } catch (e) {
            this.showToast('Failed to mark all as read', 'error')
        }
    }

    async loadPreferences() {
        try {
            const resp = await fetch('/api/preferences')
            if (!resp.ok) throw new Error('Failed to load preferences')

            const data = await resp.json()
            this.renderPreferences(data.preferences || [])
            this.loadLastDeliveries()
        } catch (e) {
            console.error('Failed to load preferences:', e)
            document.getElementById('preferences-list').innerHTML =
                '<p class="text-muted">No preferences configured</p>'
        }
    }

    renderPreferences(preferences) {
        const list = document.getElementById('preferences-list')

        if (preferences.length === 0) {
            list.innerHTML = '<p class="text-muted">No notification types available</p>'
            return
        }

        list.innerHTML = preferences.map(pref => {
            const providers = pref.providers || []
            const selectedProvider = pref.provider || ''
            const providerSelect = providers.length > 0 ? `
                <select class="provider-select" onchange="app.changeProvider('${pref.definition_code}', '${pref.channel}', this.value)">
                    <option value="">Auto</option>
                    ${providers.map(p => `<option value="${p}" ${selectedProvider === p ? 'selected' : ''}>${p}</option>`).join('')}
                </select>
            ` : '<span class="text-muted">Default provider</span>'

            const providerBadge = selectedProvider
                ? `<span class="chip chip-soft">via ${this.escapeHtml(selectedProvider)}</span>`
                : ''

            return `
            <div class="preference-item">
                <div class="preference-label">
                    <div style="font-weight: 500;">${this.escapeHtml(pref.definition_name)}</div>
                    <div class="pref-meta">
                        <span class="chip">${this.escapeHtml(pref.channel)}</span>
                        ${providerBadge}
                    </div>
                </div>
                <div class="preference-actions">
                    ${providerSelect}
                    <label class="toggle">
                        <input type="checkbox"
                               ${pref.enabled ? 'checked' : ''}
                               onchange="app.togglePreference('${pref.definition_code}', '${pref.channel}', this.checked, '${selectedProvider}')">
                        <span class="toggle-slider"></span>
                    </label>
                </div>
            </div>
            `
        }).join('')
    }

    async loadLastDeliveries() {
        try {
            const resp = await fetch('/api/deliveries/last')
            if (!resp.ok) throw new Error('Failed to load deliveries')
            const data = await resp.json()
            this.renderLastDeliveries(data.deliveries || [])
        } catch (e) {
            console.error('Failed to load deliveries:', e)
            this.renderLastDeliveries([])
        }
    }

    renderLastDeliveries(deliveries) {
        const container = document.getElementById('last-deliveries')
        if (!container) return
        if (!deliveries || deliveries.length === 0) {
            container.innerHTML = '<p class="text-muted">No deliveries yet</p>'
            return
        }
        container.innerHTML = deliveries.map(d => `
            <div class="delivery-row">
                <div>
                    <div class="delivery-title">${this.escapeHtml(d.definition_code || 'notification')}</div>
                    <div class="delivery-meta">
                        <span class="chip">${this.escapeHtml(d.channel)}</span>
                        <span class="chip chip-soft">${this.escapeHtml(d.provider)}</span>
                    </div>
                    <div class="delivery-dest text-muted">${this.escapeHtml(d.address)}</div>
                </div>
                <div class="delivery-status ${d.status === 'succeeded' ? 'status-ok' : 'status-failed'}">
                    ${d.status || 'unknown'}
                </div>
            </div>
        `).join('')
    }

    async togglePreference(definitionCode, channel, enabled, provider) {
        return this.updatePreference(definitionCode, channel, enabled, provider)
    }

    async updatePreference(definitionCode, channel, enabled, provider) {
        try {
            const payload = {
                definition_code: definitionCode,
                channel: channel,
            }
            if (enabled !== undefined && enabled !== null) {
                payload.enabled = enabled
            }
            if (provider !== undefined) {
                payload.provider = provider
            }
            await fetch('/api/preferences', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            })
            this.showToast('Preference updated', 'info')
        } catch (e) {
            this.showToast('Failed to update preference', 'error')
        }
    }

    changeProvider(definitionCode, channel, provider) {
        this.updatePreference(definitionCode, channel, null, provider)
    }

    async sendTestNotification() {
        try {
            await fetch('/api/notify/test', { method: 'POST' })
            this.showToast('Test notification sent!', 'info')
            this.loadLastDeliveries()
        } catch (e) {
            this.showToast('Failed to send test notification', 'error')
        }
    }

    async broadcastAlert() {
        const message = prompt('Enter alert message:')
        if (!message) return

        try {
            await fetch('/admin/broadcast', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    definition_code: 'system_alert',
                    context: {
                        title: 'System Alert',
                        message: message
                    }
                })
            })
            this.showToast('Alert broadcasted to all users!', 'info')
            this.loadLastDeliveries()
        } catch (e) {
            this.showToast('Failed to broadcast alert', 'error')
        }
    }

    async viewStats() {
        try {
            const resp = await fetch('/admin/stats')
            const data = await resp.json()
            alert(`Delivery Stats:\nTotal: ${data.total}\nSucceeded: ${data.succeeded}\nFailed: ${data.failed}`)
        } catch (e) {
            this.showToast('Failed to load stats', 'error')
        }
    }

    async loadUsers() {
        try {
            console.log('Loading users...')
            const resp = await fetch('/admin/users')
            if (!resp.ok) throw new Error('Failed to load users')
            const data = await resp.json()
            console.log('Users loaded:', data.users)

            const select = document.getElementById('user-select')
            if (!select) {
                console.error('user-select element not found!')
                return
            }

            // Clear and populate
            select.innerHTML = '<option value="">Select user...</option>'
            data.users.forEach(user => {
                // Exclude current user from the list
                if (user.id !== this.currentUser.id) {
                    const option = document.createElement('option')
                    option.value = user.id
                    option.textContent = `${user.name} (${user.email})`
                    select.appendChild(option)
                }
            })
            console.log('User dropdown populated with', select.options.length - 1, 'users')
        } catch (e) {
            console.error('Failed to load users:', e)
            this.showToast('Failed to load users', 'error')
        }
    }

    async sendToUser() {
        const select = document.getElementById('user-select')
        const textarea = document.getElementById('user-message')

        if (!select || !textarea) return

        const userId = select.value
        const message = textarea.value.trim()

        if (!userId) {
            this.showToast('Please select a user', 'error')
            return
        }

        if (!message) {
            this.showToast('Please enter a message', 'error')
            return
        }

        try {
            const resp = await fetch('/admin/send-to-user', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    user_id: userId,
                    message: message
                })
            })

            if (!resp.ok) throw new Error('Failed to send notification')

            this.showToast('Notification sent!', 'info')
            textarea.value = '' // Clear the message
            this.loadLastDeliveries()
        } catch (e) {
            this.showToast('Failed to send notification: ' + e.message, 'error')
        }
    }

    async loadAvailableChannels() {
        try {
            const resp = await fetch('/api/channels')
            if (!resp.ok) throw new Error('Failed to load channels')
            const data = await resp.json()

            const container = document.getElementById('active-adapters')
            if (container) {
                const adapterBadges = data.adapters.map(a =>
                    `<span style="display: inline-block; background: #3498db; color: white; padding: 2px 8px; border-radius: 3px; margin: 2px; font-size: 11px;">${a}</span>`
                ).join('')
                container.innerHTML = adapterBadges || 'None'
            }
        } catch (e) {
            console.error('Failed to load available channels:', e)
        }
    }

    updateBadge() {
        const badge = document.getElementById('unread-count')
        if (badge) {
            badge.textContent = this.unreadCount
            badge.style.display = this.unreadCount > 0 ? 'inline-flex' : 'none'
        }
    }

    showToast(message, type = 'info') {
        const container = document.getElementById('toast-container')
        const toast = document.createElement('div')
        toast.className = `toast ${type}`
        toast.textContent = message
        container.appendChild(toast)

        setTimeout(() => {
            toast.remove()
        }, 3000)
    }

    escapeHtml(text) {
        const div = document.createElement('div')
        div.textContent = text
        return div.innerHTML
    }

    formatTime(timestamp) {
        if (!timestamp) return 'Just now'
        const date = new Date(timestamp)

        // Check if date is valid
        if (isNaN(date.getTime())) return 'Just now'

        const now = new Date()
        const diff = now - date

        if (diff < 0) return 'Just now' // Future date, treat as just now
        if (diff < 60000) return 'Just now'
        if (diff < 3600000) return Math.floor(diff / 60000) + 'm ago'
        if (diff < 86400000) return Math.floor(diff / 3600000) + 'h ago'
        if (diff < 2592000000) return Math.floor(diff / 86400000) + 'd ago' // Less than 30 days

        // For older dates, show actual date
        return date.toLocaleDateString()
    }
}

// Initialize app
const app = new NotificationCenter()
document.addEventListener('DOMContentLoaded', () => {
    app.init()
})
