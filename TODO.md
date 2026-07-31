# TOOD

- [ ] Add reminder engine we can use to schedule commands to remind people, we need to coordindate with opt-out and other features
- [ ] reminders should integrate with go-options to manage options
- [ ] Modify attachment workflow so that we can define an upload service to store bytes and convert to URL before dispatch.
- [ ] update readme with export ready stuff
- [x] exportready: rename package to `onready`
- [x] exportready should use snake_case variable names in templates, i.e. OnReadyEvent should json fields tags
- [ ] How to handle multichannel dispatch messages, i.e. i want to send the same message to slack and email
- [ ] How are we handling multichannel messages? i.e. SMS is a short message, Slack has different markup, Email can be Text and HTML
- [ ] Implement adapters, right now they are just logging to stdout
- [ ] Review our internal/storage/bun/criteria.go do we need it? cant we use the repostory criteria?

You are starting fresh. First, read `NOTIF_TSK.md`, then locate **Task N.N** (change this number as needed), and read its context plus all referenced files.   Is very important that you follow the guiding notes.  Carry out every requirement and deliverable for that task—don’t skip audits, fixtures, or code updates. When everything is completed, update `NOTIF_TSK.md` to change the checkbox for **Task N.N** from `- [ ]` to `- [x]`. Before you finish, summarize what you did, mention any tests you ran (or why you didn’t), and confirm the files touched. Task or tasks to complete: Tasks for Phase 4


Review our implementation looking for bugs, logical errors, or implementation gaps

Implement fixes to address all issues identified, end to end, prefer long term solutions to hot fixes

use [$ctx](/Users/goliatone/.agents/skills/ctx/SKILL.md) to update relevant spec docs to capture the findings, gaps, recommended next action without starting to code

taskfile go:quality:all fails, fix all issues until all linters and test pass





----
Popular Services/Protocols to Consider
Email Providers:
AWS SES (Amazon Simple Email Service)
Postmark
Mailjet
SparkPost
Mandrill (Mailchimp Transactional)
SMS Providers:
AWS SNS
Vonage (formerly Nexmo)
Plivo
MessageBird
Telnyx
Push Notifications:
Firebase Cloud Messaging (FCM) - Android/iOS/Web
Apple Push Notification service (APNs) - iOS
OneSignal - Multi-platform
Pusher Beams
AWS SNS Mobile Push
Chat/Messaging Platforms:
Discord
Microsoft Teams
Google Chat (formerly Hangouts Chat)
Mattermost
Rocket.Chat
Collaboration Tools:
Webhooks (generic HTTP POST)
PagerDuty
Opsgenie
Datadog Events
Social/Community:
Twitter/X
LinkedIn
Facebook Messenger
Instagram Direct
Voice Calls:
Twilio Voice (if not already covered)
Vonage Voice API
In-App/WebSocket:
Pusher Channels
Ably
Socket.IO


Highest Priority Recommendations:
Webhooks - Generic adapter for custom integrations
Firebase FCM - Essential for mobile push notifications
AWS SES - Very popular, cost-effective email
Discord - Popular for developer communities
AWS SNS - Unified service for SMS/Push/Email
