---
name: terraform-plan-review
description: Review a terraform plan for risk and drift before apply, flagging destructive changes, IAM/security-policy edits, and resources that will be replaced rather than updated.
---

# Terraform Plan Review

Use this skill when a user asks you to review, sanity-check, or assess the risk
of a `terraform plan` before they apply.

## Procedure

1. Obtain machine-readable plan JSON: `terraform show -json plan.tfplan > plan.json`.
2. Run the analyzer: `oxid review --plan plan.json`.
3. Summarize by severity: destroy/replace, security-sensitive, drift, benign.
4. End with a one-line verdict the user (or CI) can act on.
