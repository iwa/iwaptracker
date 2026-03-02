# iwaptracker

fast and simple alert system for Archipelago, notifying useful sent items through Ntfy

## run

```yaml
services:
  iwaptracker:
    image: ghcr.io/iwa/iwaptracker:latest
    container_name: iwaptracker
    restart: unless-stopped
    volumes:
      - ./data:/app/data
    environment:
      PERIOD_MINUTES: 60
      ROOM_ID: "room suuid"
      SLOT_IDS: "1,2,3"
      NTFY_URL: "..."
      DISCORD_WEBHOOK_URL: "..."
      SIGNAL_MESSAGE_URL: "..."
      SIGNAL_NUMBER: "..."
      SIGNAL_RECIPIENT: "..."
```
