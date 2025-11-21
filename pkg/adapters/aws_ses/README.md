AWS SES Adapter
---------------
Delivers `email` channel messages via Amazon SES with text and HTML support.

Usage
- Configure region/from (optionally profile/config set):  
  `aws_ses.New(logger, aws_ses.WithConfig(aws_ses.Config{Region: "us-east-1", From: "no-reply@example.com"}))`
- You can inject a custom SES client with `WithClient`; set `DryRun` to log without sending.
- Per-message metadata: `from`, `text_body`, `html_body`, `body`, `cc`, `bcc`.

Credentials
- SES uses AWS credentials (access key/secret) in the target region. Use environment (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, optionally `AWS_SESSION_TOKEN`), shared config/credentials files, or an assumed role.
- Set AWS region (`AWS_REGION` or `WithConfig.Region`).
- SES console & getting credentials: https://docs.aws.amazon.com/ses/latest/dg/getting-started.html
- AWS IAM user/access keys: https://docs.aws.amazon.com/IAM/latest/UserGuide/id_users_create.html
