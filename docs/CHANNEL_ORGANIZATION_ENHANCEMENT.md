# CHANNEL_ORGANIZATION_ENHANCEMENT.md

## Problems

### 1. Channel navigation is broken

The Channels page navigates to `/#/?channel=gaming` but the Feed page initializes `channel` state as `''` and never reads from the URL. The channel filter pills always show "All" as active regardless of the URL.

**Fix:** The Feed page should read the `channel` query parameter from the hash URL on mount and whenever it changes. The router needs to support query params, or the Feed reads them directly from `window.location.hash`.

### 2. Channel pills don't scale

The current horizontal pill bar renders every channel. With 1,000 channels this becomes unusable — a scrollable row of 1,000 tiny buttons.

**Fix:** Replace the pill bar with a hybrid approach:

- **Top 5 channels** shown as quick-filter pills (most posts, always visible)
- **Search input** that filters/autocompletes channel names as you type
- When a channel is actively selected that's not in the top 5, it appears as an additional pill

---

## Proposed Design

```
┌─────────────────────────────────────────────────────────────────────┐
│  [All] [general] [gaming] [politics] [crypto] [memes]   [🔍 ...]  │
└─────────────────────────────────────────────────────────────────────┘
```

The `[🔍 ...]` button expands into a text input:

```
┌─────────────────────────────────────────────────────────────────────┐
│  [All] [general] [gaming ✓]                     [ga.............. ] │
│                                                  ┌───────────────┐ │
│                                                  │ gaming    (15)│ │
│                                                  │ gadgets    (3)│ │
│                                                  │ garage     (1)│ │
│                                                  └───────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

Behavior:
- Typing filters the channel list alphabetically, matching from the start of the name
- Results show channel name + post count
- Clicking a result selects it and closes the dropdown
- The selected channel appears as a pill (even if not in the top 5) with an `✕` to deselect
- Maximum 5 suggestion results shown at a time
- Empty search shows the top channels by post count

---

## Implementation Steps

### Step 1 — Fix channel URL sync

**`src/pages/Feed.tsx`**

Read the initial channel from the URL hash:

```typescript
function getChannelFromHash(): string {
  const hash = window.location.hash;
  const match = hash.match(/[?&]channel=([^&]*)/);
  return match ? decodeURIComponent(match[1]) : '';
}

const [channel, setChannel] = useState(getChannelFromHash);
```

When channel changes, update the URL:
```typescript
useEffect(() => {
  const base = '#/';
  window.history.replaceState(null, '', channel ? `${base}?channel=${channel}` : base);
}, [channel]);
```

**`src/pages/Channels.tsx`**

Change navigation from `navigate('?channel=gaming')` to setting the hash correctly:
```typescript
onClick={() => navigate(`?channel=${c.channel}`)}
// Change to:
onClick={() => { window.location.hash = `#/?channel=${c.channel}`; }}
```

### Step 2 — Channel search component

**`src/components/ChannelSearch.tsx`** (new)

Props:
```typescript
{
  channels: ChannelInfo[];
  selected: string;
  onSelect: (channel: string) => void;
}
```

Renders:
- Top 5 channel pills (by post count)
- "All" pill (always first)
- If the selected channel isn't in the top 5, show it as an additional pill with `✕`
- Search icon button that expands to a text input
- Dropdown with filtered results (matches prefix, sorted alphabetically, max 5 shown)
- Post count next to each result
- Closes on selection, escape, or click-outside

### Step 3 — Replace pill bar in Feed

**`src/pages/Feed.tsx`**

Replace the current `channels.data && channels.data.length > 0 && (...)` block with `<ChannelSearch>`.

### Acceptance Criteria

- Clicking a channel on the Channels page navigates to the Feed with that channel selected.
- Refreshing the page preserves the channel filter (it's in the URL).
- Top 5 channels are always visible as pills.
- Typing in the search narrows results alphabetically.
- Selecting from search updates the feed.
- With 100+ channels, the UI remains clean (only 5 pills + search).
