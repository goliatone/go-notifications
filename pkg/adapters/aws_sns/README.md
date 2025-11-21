AWS SNS Adapter
---------------
Delivers messages via Amazon SNS for SMS (direct phone numbers) or topic fanout (email, SMS, SQS, etc.). Supports text; HTML is stripped to text. Use the SES adapter for full email rendering.

Usage
- Configure region/profile and optional default topic ARN:  
  `aws_sns.New(logger, aws_sns.WithConfig(aws_sns.Config{Region: "us-east-1", TopicArn: "arn:aws:sns:us-east-1:123456789012:alerts"}))`
- Dry-run logging: set `DryRun: true` to log without sending.
- Per-message metadata: `topic_arn` (override), `body`, `html_body` (stripped), `subject` (used for topic email endpoints), and `to` can be a phone number for direct SMS when no topic ARN is provided.

Credentials
- SNS uses AWS credentials (access key/secret or assumed role) in the target region.
- Configure via environment (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`), shared config/credentials files, or profile (`WithConfig.Profile`).
- Set AWS region (`AWS_REGION` or `WithConfig.Region`).
- SNS getting started: https://docs.aws.amazon.com/sns/latest/dg/sns-getting-started.html
