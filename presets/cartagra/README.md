# Cartagra HD Preset

This preset records the Cartagra HD workflow used for Vietnamese localization.

Pass your local Cartagra install path through `-GameDir`.

## Script Decompile

```powershell
.\presets\cartagra\decompile.ps1 `
  -GameDir '<path-to-Cartagra>' `
  -OutDir '.\presets\cartagra\out\preview_multilang'
```

The preset uses:

- `data\CartagraHD.txt`
- `data\CartagraHD_v1.py`
- `SCRIPT.PAK`
- UTF-8 export
- `--no_subdir`

The Python parser handles:

- `MESSAGE`: Japanese UTF-16 plus English UTF-8.
- `LOG_BEGIN`: Japanese, English, and Simplified Chinese UTF-16.
- `DIALOG`: raw control-read fix.

## Script Import

```powershell
.\presets\cartagra\import.ps1 `
  -GameDir '<path-to-Cartagra>' `
  -InputDir '<path-to-edited-scripts>'
```

The script backs up the target `SCRIPT.PAK` before replacing it.

## Font Patch

Cartagra HD uses two relevant font groups:

- Main dialog: `files\font_win32_1920\FONT0*.PAK`
- Prologue/opening/system text: `files\SYSFONT.PAK`

The final Vietnamese font approach is hybrid:

- Copy the base Latin glyph from the game's bitmap atlas.
- Render Vietnamese marks from a Windows TTF.
- Keep metrics copied from the base glyph so engine layout stays stable.

Recommended current patch:

```powershell
.\presets\cartagra\patch-font-segoe.ps1 `
  -GameDir '<path-to-Cartagra>' `
  -OutputRoot '.\presets\cartagra\out\font_patch_segoe' `
  -Install
```

Alternative Noto patch:

```powershell
.\presets\cartagra\patch-font-noto.ps1 `
  -GameDir '<path-to-Cartagra>' `
  -OutputRoot '.\presets\cartagra\out\font_patch_noto' `
  -Install
```

The current preferred result is Segoe UI marks with a special placement tweak for `ư`, plus separate stroke placement for `đ` and `Đ`.

## Tool Notes

Additional tools added under `tools/`:

- `tools/compositevietfont`: final Cartagra bitmap-composite font patcher.
- `tools/sysfontpatch`: older TTF injection patcher for combined `SYSFONT.PAK`.
- `tools/vietfontpatch`: older TTF injection patcher for split font packs.
- `tools/fontsample`: renders a quick PNG sample from an info/glyph pair.

Build examples:

```powershell
go build ./tools/compositevietfont
go build ./tools/fontsample
go build ./tools/sysfontpatch
go build ./tools/vietfontpatch
```
