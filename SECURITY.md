# Security and sensitive material

Do not put access tokens, wallet keys, personal data, production logs, or private
engine code in public issues or pull requests. Use GitHub's private vulnerability
reporting feature for a suspected security issue affecting a published component.

Public examples do not need production credentials. Use a separate disposable
database for tests. The astronomy service's development secret must never be
reused as an application, wallet, or production infrastructure credential.

If you suspect a disclosure, stop further publication and notify the repository
owner privately. Deleting a public file does not remove copies or Git history.
