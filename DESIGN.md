---
name: Code Central
description: A local coordination point for agent-driven delivery, drawn as a midnight transit diagram.
colors:
  midnight-enamel: "#12224a"
  rail-lit: "#213a76"
  rail-text: "#e8ebf4"
  rail-muted: "#aeb8d4"
  rail-rule: "rgb(255 255 255 / .12)"
  porcelain: "#f4f2ec"
  panel-white: "#ffffff"
  linen: "#ebe8df"
  ink: "#14182a"
  slate: "#555a6a"
  hairline: "#dcd8cc"
  cobalt-signal: "#2a5bd7"
  danger: "#c0302a"
  primary: "#12224a"
  on-primary: "#ffffff"
  on-line: "#ffffff"
  line-answer: "#e39b12"
  line-answered: "#7a4bc8"
  line-silent: "#6b7385"
  line-long: "#4b566c"
  line-failed: "#d4382d"
  line-review: "#2a9457"
  line-landing: "#10847a"
  line-blocked: "#a8741a"
  line-external: "#1b84a3"
  state-draft: "#8b8f99"
  state-ready: "#5b6b8f"
  state-in-progress: "#2f62d8"
  state-done: "#1f7a45"
  state-cancelled: "#8b8f99"
  ribbon-1: "#f08a6c"
  ribbon-2: "#5aa9f0"
  ribbon-3: "#e8bf4a"
  ribbon-4: "#6cc77f"
  ribbon-5: "#c18ae6"
  ribbon-6: "#5fd0cb"
  night-rail: "#060c20"
  night-surface: "#0e1833"
  night-panel: "#152246"
  night-linen: "#1d2d57"
  night-text: "#eef0f6"
  night-primary: "#8fb0ff"
  night-on-primary: "#0a1430"
typography:
  display:
    fontFamily: "Hanken Grotesk Variable, system-ui, -apple-system, sans-serif"
    fontSize: "32px"
    fontWeight: 600
    lineHeight: 1.25
  headline:
    fontFamily: "Hanken Grotesk Variable, system-ui, -apple-system, sans-serif"
    fontSize: "19px"
    fontWeight: 700
    letterSpacing: "-0.01em"
  title:
    fontFamily: "Hanken Grotesk Variable, system-ui, -apple-system, sans-serif"
    fontSize: "17px"
    fontWeight: 800
    letterSpacing: "-0.01em"
  body:
    fontFamily: "Hanken Grotesk Variable, system-ui, -apple-system, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.45
  label:
    fontFamily: "Hanken Grotesk Variable, system-ui, -apple-system, sans-serif"
    fontSize: "11px"
    fontWeight: 700
    letterSpacing: "0.12em"
  mono:
    fontFamily: "Red Hat Mono Variable, ui-monospace, Menlo, monospace"
    fontSize: "12px"
    fontWeight: 600
    fontFeature: "tnum"
rounded:
  tag: "4px"
  control: "6px"
  popover: "8px"
  panel: "10px"
  modal: "12px"
  block: "16px"
  pill: "999px"
spacing:
  hair: "4px"
  xs: "6px"
  sm: "8px"
  md: "12px"
  lg: "16px"
  xl: "24px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.control}"
    padding: "5px 12px"
  button-secondary:
    backgroundColor: "{colors.panel-white}"
    textColor: "{colors.ink}"
    rounded: "{rounded.control}"
    padding: "5px 12px"
  button-on-rail-primary:
    backgroundColor: "{colors.porcelain}"
    textColor: "{colors.midnight-enamel}"
    rounded: "{rounded.control}"
    padding: "5px 12px"
  rail-filter-on:
    backgroundColor: "{colors.rail-lit}"
    textColor: "{colors.panel-white}"
    rounded: "{rounded.control}"
    padding: "6px 8px"
  view-head:
    backgroundColor: "{colors.midnight-enamel}"
    textColor: "{colors.rail-text}"
    height: "56px"
    padding: "10px 20px"
  panel:
    backgroundColor: "{colors.panel-white}"
    rounded: "{rounded.panel}"
    padding: "14px 16px"
  board-card:
    backgroundColor: "{colors.panel-white}"
    textColor: "{colors.ink}"
    rounded: "{rounded.control}"
    padding: "10px 12px"
  state-chip:
    textColor: "{colors.ink}"
    rounded: "{rounded.pill}"
    padding: "1px 8px 1px 5px"
    typography: "{typography.label}"
  input:
    backgroundColor: "{colors.panel-white}"
    textColor: "{colors.ink}"
    rounded: "{rounded.control}"
    padding: "6px 8px"
---

# Design System: Code Central

## Overview

**Creative North Star: "The Midnight Transit Diagram"**

Work moves along fixed routes like a metro network. States are lines, Stages are stations, and Needs Me is the interchange where every line meets the human. The UI is a wayfinding diagram, not a tracker: a midnight-blue enamel rail holds the brand, the views and the State keys, beside a porcelain work surface where the lines are drawn in their own inks. Colour is never decoration here; every saturated stroke is a line a piece of work is travelling on.

Density is that of a signboard read at a glance: compact 14px text, hairline rules between rows, mono ids and counts with tabular figures, and the line inks carrying the meaning so the eye finds what waits without reading. The diagram's two marks do almost all of the work: a **line** (a thick coloured stroke, 3 to 5px) and a **station** (a surface-coloured disc inside a thick ring of the line's ink). Group heads, Board column heads, the Stage strip, Needs Me groups, State chips and the Map's nodes are all built from those two marks.

The system refuses the neutral grey tracker with a single blue accent. Dark mode is not an inversion: the rail goes deeper (night enamel), the surface becomes night blue, and the same lines are relit brighter so they keep their identity.

**Key Characteristics:**
- Midnight enamel rail (and the same enamel laid across the top of full-width views) beside a porcelain work surface.
- Every State and every Needs Me reason has its own line ink; the ink is the meaning.
- Stations: a ring of line ink around a surface-coloured disc; the selected or current one grows, and fills or turns to the text colour.
- Lines as strokes: 3px under group heads, 4px down a Needs Me group, 5px under Board column heads and along the Stage strip.
- Hanken Grotesk for text, Red Hat Mono with tabular figures for ids, counts and times.
- Flat surfaces with hairline borders; shadows only on things that float.

## Colors

A midnight-and-porcelain base with a full set of transit line inks, each owned by exactly one meaning.

### Primary
- **Midnight Enamel** (`midnight-enamel`): the rail, the `.view-head` band across full-width views, and the primary button fill on the work surface. It is the shell of the whole UI, theme-invariant in character (night enamel in dark).
- **Rail Lit** (`rail-lit`): the selected filter on the rail, and its hover wash at 55%.
- **Porcelain** (`porcelain`): the work surface background, and the fixed fill of a primary button sitting on the rail or the view head (with Midnight Enamel text), in both themes.

### Secondary
- **Cobalt Signal** (`cobalt-signal`): links, focus outlines, caret, selection wash, the running Stage on the strip, the selected tab underline, and Requirement/Prototype badges. It is a signal, not a brand fill.

### Tertiary: line inks
The reason lines (`line-*`) colour Needs Me groups and their stations, a needing Stage, a needing Map node and a needing Board card:
- **Answer Amber** (`line-answer`): Needs Answer; the loudest line, also the Round panel's outline and the Needs Me count.
- **Answered Violet** (`line-answered`), **Failed Scarlet** (`line-failed`, also CI failed, base diverged/missing), **Review Green** (`line-review`, also done Stages and passed CI), **Landing Teal** (`line-landing`, to merge / accepted not landed), **Blocked Ochre** (`line-blocked`), **External Cyan** (`line-external`, Incoming Issues), **Silent Grey** (`line-silent`) and **Long Slate** (`line-long`).

The State lines (`state-*`) colour the rail's State keys, State chips and Board column heads: **Draft Grey**, **Ready Slate-Blue**, **In Progress Cobalt**, **Done Green**, **Cancelled Grey**. The workflow States Needs Answer, Answered, Failed, In Review and Accepted reuse their reason's line, so one State keeps one ink everywhere.

**Project ribbons** (`ribbon-1` to `ribbon-6`): a separate pastel palette, assigned by a hash of the Project's Key so a Project keeps its colour everywhere. Shared by both themes because it sits on the rail.

### Neutral
- **Panel White** (`panel-white`): panels, cards, inputs, menus, rows when selected.
- **Linen** (`linen`): the secondary surface: code, control groups, Board cells (at 55%), badges.
- **Ink** (`ink`): body text. **Slate** (`slate`): muted text and secondary labels. **Hairline** (`hairline`): every 1px border and row rule.
- **Rail Text / Rail Muted / Rail Rule**: text, secondary text and the 12% white edge on the midnight shell.
- **Danger** (`danger`): destructive buttons, warnings and the error banner (as a 12% wash with danger text).
- Dark theme: `night-rail`, `night-surface`, `night-panel`, `night-linen`, `night-text`, with `night-primary` as the primary fill and `night-on-primary` as text on it and on filled lines. Every line ink has a brighter dark counterpart in `styles.css`, defined twice (for `prefers-color-scheme` and for `data-theme="dark"`); the two copies must stay identical.

### Named Rules
**The One Line, One Ink Rule.** Every State and every reason owns one line ink, and that ink means only that line. Project ribbons have their own palette and never borrow a line ink.

**The Readable Ink Rule.** Line ink is never set as text at full strength. Text in a line's colour is `color-mix(in srgb, <line> 55%, var(--text))`, so it passes contrast in both themes; full-strength ink is for strokes, rings and fills only.

**The Filled Line Rule.** Text on a filled line colour (the Activity Dashboard's State blocks) uses `--on-line`: white in light, near-black navy in dark.

## Typography

**Display Font:** Hanken Grotesk Variable (with system-ui, -apple-system, sans-serif), self-hosted via @fontsource-variable.
**Label/Mono Font:** Red Hat Mono Variable (with ui-monospace, Menlo, monospace), self-hosted.

**Character:** A humanist grotesk that reads like station signage at small sizes, paired with one mono whose tabular figures keep ids, counts and waited times aligned in columns.

### Hierarchy
- **Display** (600, 32px, 1.25): Focus Mode's item title and Question; Focus's All clear is 40px. Focus Mode scales everything up (18px base, panels zoomed 1.3).
- **Headline** (700, 19px): a Ticket's or Requirement's title in the detail pane.
- **Title** (800, 17px; the rail's brand at 16px): the view head's name and the brand, each after the roundel.
- **Body** (400, 14px, 1.45): everything else; rows 500 weight, selected row 700. Details and meta at 12.5px.
- **Label** (700, 11px, 0.12em, uppercase): group heads and Board column heads, set in their line's readable ink. Rail section headings are 10.5px at 0.14em in Rail Muted.
- **Mono** (600, 12px, tabular): ids (`#19`), counts in group heads, column heads and on the rail.

### Named Rules
**The Tabular Figures Rule.** Any number a user compares (ids, counts, waited times, percentages) is set with tabular figures, in the mono where it is an id or a count.

**The Signage Label Rule.** Uppercase tracked labels name a line (a reason group, a State column) or a rail section, and nothing else. They are never decorative lead-ins above a heading.

## Layout

The Inbox is a three-column grid: the 14rem rail, a 24rem list, and the detail pane filling the rest, full viewport height with each column scrolling on its own. Full-width views (Board, Focus, Activity, CI, Usage, Options) stack a 56px midnight view head over a body that is one centred column (1100px max, 24px padding, 24px gaps); Focus Mode's item column is 760px; Project Options is 60rem with a sticky 9rem section menu from 900px up.

The Board is a grid of State columns (min 165px each, 8px gaps) under sticky heads, scrolling sideways once the columns reach their minimum; By Spec lanes put a sticky Lane header bar across the Board.

Spacing runs on 4 / 6 / 8 / 12 / 16 / 24px. Rows are 9px by 14px with a hairline between; panels pad 14px by 16px.

Below 900px the Inbox becomes one column: the rail wraps into a horizontal band at the top, each rail group starting its own line under its heading.

## Elevation & Depth

Flat by default. Depth comes from the tonal step between rail, porcelain surface, white panels and linen insets, and from hairline borders. Shadows belong only to things that float over the page: menus and tooltips, the + New menu, modals, the Chat icon and pop-up, and the Limits popup.

### Shadow Vocabulary
- **Float** (`--shadow`: `0 6px 20px rgb(18 34 74 / .14)`, dark `0 8px 28px rgb(0 0 0 / .45)`): the Chat icon.
- **Menu** (`0 4px 16px rgb(0 0 0 / .12)` to `0 8px 24px rgb(0 0 0 / .14)`): row menus, Factory tooltip, + New menu, Columns menu.
- **Modal** (`0 12px 40px rgb(0 0 0 / .25)`): the modal card over a 35% black scrim.
- **Station halo** (`0 0 0 4px color-mix(in srgb, <line> 25%, transparent)`): a soft ring around a station or Map node that waits on the human. It is a signal, not elevation.

### Named Rules
**The Flat Surface Rule.** Nothing resting on the page casts a shadow: rows, cards, panels and columns are separated by tone and hairlines. A shadow means the thing floats.

## Shapes

Square-ish: 6px corners on buttons, inputs, cards, Map nodes and menu items; 4px on tags and badges; 8px on menus, popovers and control groups; 10px on panels and the Chat pop-up; 12px on the modal; 16px on the Activity Dashboard's State blocks. Pills (999px) are reserved for State chips, and circles for stations, the roundel and the Factory dot. Borders are 1px hairlines; coloured strokes are the lines (3 to 5px) and station rings (2.5 to 5px).

The roundel, Code Central's mark, is an interchange ring (14px, 4px stroke in the current colour) before the brand on the rail and before each view head's title.

### Named Rules
**The Ring Is a State Rule.** A ring means a station on a line. Project ribbons are bars (4 by 16px, 2px corners), never rings, so a Project never reads as a State.

## Components

### Buttons
Quiet, firm, signage-weight (600).
- **Shape:** gently squared (6px), 5px by 12px.
- **Primary:** Midnight Enamel fill, white text; on hover it mixes 14% toward Cobalt Signal. In dark, Night Primary with near-black text.
- **Secondary:** white with a hairline border; the border darkens to Slate on hover. **Danger:** danger text with a 40% danger border.
- **On the rail and view head:** primary turns Porcelain with Midnight text (hover pure white); secondary goes transparent with a 30% Rail Text outline that firms on hover. Focus outlines there are Porcelain.
- **Split + New:** primary main part and a caret part, divided by a 35% currentColor rule.

### Chips
- **State chip:** a pill with a station ring before the State's name, on a 12% wash of the State's line with a 35% border. The current State (`here`) washes to 24%, takes a full-ink border and fills its station.
- **Badges** (kind, stage, CI, label): 4px corners, 10.5 to 11px, outlined in the badge's colour or on linen; uppercase only for kind/stage.

### Cards / Containers
- **Panel:** white, hairline border, 10px corners, 14px by 16px. The Round panel is outlined 1.5px in Answer Amber, its head carrying an amber station.
- **Board card:** white, hairline border, 6px corners, 10px by 12px; border to Slate on hover. A card that needs the human is ringed all round in its reason's ink (1px border plus 1px ring shadow).
- **Ticket controls:** a linen inset, 8px corners, on a subgrid.

### Inputs / Fields
- **Style:** white fill, hairline border, 6px corners, 6px by 8px; caret in Cobalt Signal. Invalid fields take a Danger border.
- **Focus:** 2px Cobalt Signal outline at 2px offset (Porcelain on the midnight shell).

### Navigation
- **The rail:** midnight enamel, 18px by 10px padding, a 1px Rail Rule inner edge. Brand with roundel; + New and Focus; sections (Projects, View, Admin) under Rail Muted signage labels. Filters are 6px-cornered rows; the selected one is Rail Lit and 700 white; counts are mono in Rail Muted, the Needs Me count in amber. State filters carry a station ring on the rail's own colour; Project filters carry their ribbon bar.
- **The view head:** the rail laid across the top: 56px, midnight enamel, roundel before the title, rail-styled controls and segmented toggles (pressed = Porcelain).
- **Tabs:** muted text, a 2px Cobalt Signal underline when selected. **Segmented toggle:** hairline group, pressed segment in primary.

### Needs Me lines (signature)
Each reason group is one line. Its head is the reason's name in readable ink over a 3px stroke of the line; from it a 4px vertical line drops down the group, through a 14px station (3.5px ring on white) on every row, and ends at the last station. The selected row's station grows to 18px and fills with the line's ink inside a surface-coloured inner ring.

### Board column heads
Each column head is uppercase signage over a 5px stroke of its State's line, with a mono count; the Needs Me column's stroke is Answer Amber and its cells take a 7% amber tint.

### The Stage strip (signature)
A Requirement's route: one line, a station per Stage. Each Stage is a 5px top stroke with a 15px station (4px ring) at its start; the line ends at the last station. Done stretches are Review Green, the running one Cobalt Signal, a queued one dashed Cobalt, one that needs the human its reason's ink with a halo and bold readable-ink label, untouched ones Hairline; skipped Stages are struck through. The Stage being shown is an interchange: a 19px ring in the text colour.

### Map nodes
Ticket nodes are 6px-cornered white boxes outlined 1.5px in their progress colour, with a 10px station before the uppercase type; queued nodes dash, needing nodes halo, cancelled nodes fade and strike through.

### Icons
An authored 16-unit SVG set drawn in the diagram's own strokes (2px, round caps, currentColor): Station (Focus), Caret, More (three stops on a faint line) and Blocked (a barred ring). Icons are decorative and hidden; their control carries the name.

## Do's and Don'ts

### Do:
- **Do** mark every State and reason with its station ring or line stroke in its own ink, and reuse the reason's ink for the matching workflow State.
- **Do** set line-coloured text as `color-mix(in srgb, <line> 55%, var(--text))`; keep full-strength ink for strokes, rings and fills.
- **Do** draw sequence as a line with stations: Stage routes, Needs Me groups, dashboard entries riding their block's line.
- **Do** put full-width views under the 56px midnight view head with the roundel before the title.
- **Do** use the mono with tabular figures for ids, counts and times.
- **Do** keep both dark token copies (`prefers-color-scheme` and `data-theme="dark"`) identical when adding a token.
- **Do** draw new icons in the Icon.tsx stroke language (16-unit viewBox, 2px round strokes, currentColor).

### Don't:
- **Don't** mark a card or row with a thick coloured side border; a needing card is ringed all round in its reason's ink, and a row's line runs through its station.
- **Don't** draw a Project as a ring or in a line ink; Projects are ribbon bars from their own palette.
- **Don't** introduce a single-accent grey tracker look, or give one line's ink a second meaning.
- **Don't** shadow anything that rests on the page; shadows are for menus, popovers, modals and the Chat pop-up.
- **Don't** use glyph or emoji icons, or a system font for display text.
- **Don't** add uppercase tracked lead-ins above headings; tracked uppercase is for line and rail section names.
