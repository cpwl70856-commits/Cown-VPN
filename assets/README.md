# Assets

Place application icons here for Fyne packaging:

| File | Purpose |
|------|---------|
| `Icon.png` | App icon (recommended ≥1024×1024 PNG) |
| `Icon.svg` | Optional vector source |

## Usage with Fyne

```bash
# From the repo root, after installing fyne CLI:
fyne package -os linux -icon assets/Icon.png -name mywgclient -executable mywgclient
fyne package -os darwin -icon assets/Icon.png -name mywgclient
fyne package -os windows -icon assets/Icon.png -name mywgclient
```

If no custom icon is provided, Fyne falls back to its default theme icon.
A placeholder `Icon.png` is not shipped — add your brand artwork before release packaging.
