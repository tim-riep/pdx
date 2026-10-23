# Canvas prototype (phase 2)

Comparison of Svelte and React for the designer canvas. Both draw the
sample invoice from the engine's area tree with the same core
(`core/`). Only the state and the overlay (hover, selection, handles, dragging)
belong to the framework. The prototype will be removed after the decision.

```bash
go run ./prototype/canvas/export
```

```bash
npm --prefix prototype/canvas ci --ignore-scripts
```

```bash
npm --prefix prototype/canvas run build
```

```bash
npm --prefix prototype/canvas run preview
```

Then open `http://localhost:5180/index.html?suite=1`. The page measures
both frameworks with 3 and with 30 pages (`?x=10`) and shows an overview.
`svelte.html?bench=1` and `react.html?bench=1` measure them individually.

The measurements are frames per second for zoom, scrolling, hover and
dragging (one step per frame), the time from the event to the DOM change
(commit) and to the next frame.
