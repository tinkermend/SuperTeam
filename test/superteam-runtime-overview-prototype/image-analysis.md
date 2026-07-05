# SuperTeam Runtime Overview Image Analysis

## Visual Target

- Source frame: 1536 x 1024 desktop console.
- Overall style: enterprise control-plane dashboard, v3 Soft-Flat, high-density but calm.
- Palette: near-white cold gray background, white translucent cards, black active buttons, blue selected/focus state, semantic green/orange/red/gray status dots.
- Typography: PingFang SC-like Chinese UI type, bold page title, compact semibold labels, tabular numeric feel in metrics.
- Layout: fixed left sidebar, 64px topbar, large central isometric operations map, right rail with overview and employee detail, floating request composer.

## Key Elements

- Left shell: SuperTeam logo, linear navigation icons, black active item, orange approval badge, bottom collapse affordance.
- Top shell: search box, environment selector, notification badge, help icon, user profile.
- Main header: title, subtitle, live status dot, five-state legend.
- Map controls: map/table segmented control, floor filter buttons, helper text.
- Central map: isometric office floor with six teams, team cards, employees, status dots, route line, idle pills and zoom controls.
- Right rail: operation overview card and selected digital employee task card.
- Bottom composer: demand input card with project selector, priority selector, attachment and primary action.

## Current Prototype Strategy

- This iteration is a **visual-lock prototype**. It uses the full reference image as a 1536 x 1024 visual layer so the final visual effect can be validated first.
- A transparent hotspot layer sits above the visual layer. It contains 19 employee hotspots from `employeePositions` and 6 team hotspots from `teamCards`.
- Hotspots expose data attributes such as `data-employee-id`, `data-team-id`, `data-status`, `data-x`, and `data-y`, and update hidden selected employee/team state when clicked.
- This deliberately separates the question “can we reach the target visual?” from the next question “how do we rebuild this as a maintainable production map/data UI?”.

## Production Direction For Next Phase

- Replace the full screenshot visual layer with a clean base-map asset or a renderer-generated office topology that contains no live team text, employee avatars, counts or selected states.
- Keep employee/team presence as coordinate-driven data: `{ employeeId, teamId, x, y, avatar, status, role, task }`.
- Render live team cards, selected employee ring, status dots, idle pills and right detail panel from data after the visual target is accepted.
- If zoom/pan or many employees are needed, move the hotspot model into a virtualized canvas/Konva/map-engine layer while preserving the same data contract.
