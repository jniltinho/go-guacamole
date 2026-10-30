# Spec Delta

## Purpose

Defines how GoGuacamole themes look and behave: the shared design tokens every theme provides (color, elevation,
motion), the interaction effects built on them, each theme's signature effects, and the accessibility and
regression guarantees that hold in every theme.

## ADDED Requirements

### Requirement: Theme catalogue
The UI SHALL offer the 16 themes `light`, `dark`, `noc`, `dracula`, `bionexo`, `matrix`, `xterm`, `macmodern`,
`ubuntu`, `kde`, `google`, `contrast`, `win11`, `xp`, `win98` and `macos`, selectable by every user.

#### Scenario: All themes selectable
- **WHEN** a user opens the theme picker
- **THEN** the 16 themes are listed and each one applies immediately when chosen

### Requirement: Token contract
Every theme SHALL resolve, after shared defaults are applied, the complete token set: colors (background, surface,
raised surface, overlay, foreground, muted, border, accent and its foreground, success, danger, warning, info,
protocol colors, focus, selection), an elevation scale of at least four levels, motion durations and easings, and
fonts — both on the page and in the theme picker swatches. A theme whose resolved set misses a token SHALL fail the
token check.

#### Scenario: Complete token set
- **WHEN** the theme token test runs over all themes
- **THEN** every theme defines every required token and the test passes

#### Scenario: Missing token
- **WHEN** a theme omits the elevation level 3 token and the shared defaults do not provide it
- **THEN** the token test fails naming the theme and the token

### Requirement: Corners
Corners SHALL be square by default. A theme MAY opt into a subtle radius of at most 8px through the radius token,
applied only by shared components (cards, buttons, inputs, badges, menus, dialogs, toasts); retro themes (`win98`,
`xp`, `macos`, `xterm`, `matrix`, `contrast`) SHALL stay square.

#### Scenario: Subtle radius only through the token
- **WHEN** the theme tests read every theme
- **THEN** no theme declares `border-radius` directly and any radius token is at most 8px

#### Scenario: Retro themes stay square
- **WHEN** the `win98` theme is active and the computed `border-radius` of cards and buttons is read
- **THEN** it is `0px`

### Requirement: Undecorated remote session
While a remote session is shown, no theme decoration, effect, transition or text effect SHALL apply to the session
display or its ancestors, and effect settings SHALL NOT alter the scaling and pointer mapping of the remote display.

#### Scenario: Remote session untouched
- **WHEN** the Matrix theme is active and a remote session is open
- **THEN** no rain, glow, text shadow, blur, shadow or transition from the theme is rendered over or inside the
  session display

#### Scenario: Pointer mapping intact
- **WHEN** a remote session is resized with Effects full, reduced and off
- **THEN** the display scales to the viewport and a click lands on the same remote coordinates in every mode

### Requirement: Volume through elevation
Surfaces SHALL express depth with the theme's elevation scale: page background lowest, then panels and cards,
then raised elements (sticky headers, sidebar, topbar), then overlays (menus, popovers, modals, toasts). Hovering an
interactive card or row SHALL raise it by one level; pressing a button SHALL visibly depress it. Overlays SHALL sit
above a dimmed backdrop.

#### Scenario: Modal above content
- **WHEN** a confirmation dialog opens
- **THEN** it uses the highest elevation level and the page behind it is dimmed

#### Scenario: Hover lift
- **WHEN** the pointer hovers a connection card
- **THEN** the card shows the next elevation level and returns to its level when the pointer leaves

### Requirement: Motion and micro-interactions
Interface transitions (route change, panel and list appearance, popover/menu/modal enter and exit, toasts, button
press, focus ring, skeleton shimmer) SHALL use the theme's motion tokens, animate only opacity and transform, and
complete within 400 ms (the shimmer loops while loading). Loading lists SHALL show skeleton placeholders instead of an empty area, and transient results (saved,
copied, failed) SHALL be shown as toasts that are announced to assistive technology and dismissible.

#### Scenario: Skeleton while loading
- **WHEN** the connections list is loading
- **THEN** skeleton rows with the table's shape are shown until data arrives

#### Scenario: Toast announced
- **WHEN** settings are saved
- **THEN** a toast appears in a live region, is read by a screen reader and disappears or can be closed

### Requirement: Effects preference and reduced motion
Each user SHALL be able to choose **Effects: full, reduced or off** in Settings, stored with their other
preferences. `reduced` SHALL keep opacity fades but remove movement, blur and decorative animations; `off` SHALL
remove all transitions, animations, blur and decorative backgrounds. When the user has made no choice, the
operating system's `prefers-reduced-motion: reduce` SHALL select `reduced`.

#### Scenario: Reduced motion by default
- **WHEN** the browser reports `prefers-reduced-motion: reduce` and the user never chose a value
- **THEN** no element moves or scales during transitions and the Matrix rain does not animate

#### Scenario: Preference changed mid-animation
- **WHEN** the user switches Effects from full to off while a menu is opening
- **THEN** the menu ends in its final state at once and no further movement occurs

#### Scenario: Effects off persisted
- **WHEN** a user selects Effects off and signs in from another browser
- **THEN** no transitions, animations or blur are rendered there either

#### Scenario: Invalid preference
- **WHEN** the preferences API receives `effects: "extreme"`
- **THEN** it answers 422 and the stored preference is unchanged

### Requirement: Signature effects per theme
Each theme SHALL keep its current identity through effects scoped to that theme: translucent bars for `macmodern`,
mica-like surfaces for `win11`, bevels and title bars for `xp` and `win98`, the classic black-and-white System 7 look
for `macos`, twm title bars on grey for `xterm`, accent glow for `dracula` and `noc`, and the character rain for
`matrix`. Signature effects SHALL follow the Effects preference and SHALL NOT reduce text contrast below the
accessibility thresholds.

#### Scenario: Signature effect follows preference
- **WHEN** the `macmodern` theme is active with Effects off
- **THEN** the top bar is opaque (no blur) and still meets the contrast thresholds

### Requirement: Accessible contrast in every theme
In every theme and Effects mode, all normal-size text (body, secondary, labels, buttons, badges, status and error
messages, sidebar, selection) SHALL have a contrast ratio of at least 7:1 against what is actually behind it, large
text at least 4.5:1, and input borders, focus indicators and status colors used as the only cue at least 3:1 against
adjacent colors. Keyboard focus SHALL always be visible, including on inputs.

#### Scenario: Contrast test
- **WHEN** the contrast tests run over every theme's used color combinations and rendered key screens
- **THEN** all combinations meet their threshold, or the test fails naming theme, combination and ratio

#### Scenario: Legacy background
- **WHEN** the `win98` theme shows a label on its desktop background
- **THEN** the label sits on an opaque surface where its text reaches at least 7:1

#### Scenario: Visible focus
- **WHEN** the user tabs through the login form in any theme
- **THEN** each focused control shows a focus indicator with at least 3:1 contrast

### Requirement: Visual regression
The project SHALL keep reference screenshots of the login, dashboard, connections, connection edit and settings
screens in every theme (effects off, fixed browser, fonts, locale, device pixel ratio and data), and CI SHALL fail
when more than 0.2 % of pixels differ until the references are intentionally updated, publishing the differences.

#### Scenario: Unintended change caught
- **WHEN** a change alters the `xp` card title bar
- **THEN** the screenshot job fails for `xp` and shows the difference

### Requirement: Performance budget
Theme effects SHALL NOT trigger layout during animations, SHALL keep the 95th-percentile frame time at or below
16.7 ms in the reference performance profile defined in the design, and the compiled theme CSS SHALL stay within
the budget defined in the design.

#### Scenario: Smooth list hover
- **WHEN** the pointer moves over 200 rows of the connections table with Effects full
- **THEN** the trace shows no layout caused by the hover effect and the p95 frame time is at most 16.7 ms

### Requirement: Terminal color schemes
SSH sessions SHALL use a terminal palette chosen by the user (Dracula, Solarized, Gruvbox, Nord, One Dark, Monokai,
Tango, xterm, Campbell, Ubuntu, Breeze and palettes matching the UI themes) or, by default, the palette that matches
the user's UI theme. A connection whose own color scheme is set SHALL override the user's choice.

#### Scenario: Follow the UI theme
- **WHEN** a user with the `noc` theme and no terminal preference opens an SSH session
- **THEN** guacd receives the NOC palette as the session's color scheme

#### Scenario: Connection override
- **WHEN** a connection's color scheme is `nord` and the user prefers `dracula`
- **THEN** the session uses the Nord palette

#### Scenario: Unknown palette
- **WHEN** the preferences API receives `terminalScheme: "pink"`
- **THEN** it answers 422
