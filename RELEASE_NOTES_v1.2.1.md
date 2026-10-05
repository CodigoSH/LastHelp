**Stored secrets show as dots**

### Changed
- A saved token or secret — a notification channel's token, an organisation's forge token and its webhook secret — is shown as dots: it is there, and which one it is cannot be seen, and there is no eye to reveal it. Write over the dots to replace it, empty the field to remove it, or leave the dots to keep it. The checkbox that removed a channel's token is gone.
- An organisation's forge token and webhook secret are changed one at a time: replacing one leaves the other, and an empty field keeps what is stored. Removing the token disconnects the organisation from its forge.

### Fixed
- Saving only the forge token, as the setup wizard does, no longer empties a webhook secret that was already stored.
