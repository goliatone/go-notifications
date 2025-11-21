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

        if (this.currentUser.admin) {
            document.getElementById('admin-panel').style.display = 'block'
            this.loadUsers()
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

        list.innerHTML = preferences.map(pref => `
            <div class="preference-item">
                <div class="preference-label">
                    <div style="font-weight: 500;">${this.escapeHtml(pref.definition_name)}</div>
                    <div style="font-size: 12px; color: #7f8c8d;">${pref.channel}</div>
                </div>
                <label class="toggle">
                    <input type="checkbox"
                           ${pref.enabled ? 'checked' : ''}
                           onchange="app.togglePreference('${pref.definition_code}', '${pref.channel}', this.checked)">
                    <span class="toggle-slider"></span>
                </label>
            </div>
        `).join('')
    }

    async togglePreference(definitionCode, channel, enabled) {
        try {
            await fetch('/api/preferences', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    definition_code: definitionCode,
                    channel: channel,
                    enabled: enabled
                })
            })
            this.showToast('Preference updated', 'info')
        } catch (e) {
            this.showToast('Failed to update preference', 'error')
        }
    }

    async sendTestNotification() {
        try {
            await fetch('/api/notify/test', { method: 'POST' })
            this.showToast('Test notification sent!', 'info')
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
            const resp = await fetch('/admin/users')
            if (!resp.ok) throw new Error('Failed to load users')
            const data = await resp.json()

            const select = document.getElementById('user-select')
            if (!select) return

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
        } catch (e) {
            this.showToast('Failed to send notification: ' + e.message, 'error')
        }
    }

    updateBadge() {
        const badge = document.getElementById('unread-count')
        if (badge) {
            badge.textContent = this.unreadCount
            badge.style.display = this.unreadCount > 0 ? 'inline-block' : 'none'
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
