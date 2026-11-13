"use client";

// Our own dropdown in place of the browser's <select>, which cannot be styled
// (the open list is drawn by the OS). ARIA "select-only combobox": focus stays
// on the button, the active option is announced via aria-activedescendant.
// Keyboard: arrows, Home/End, Enter/Space, Escape, type-ahead by first letters.
// With `name` it keeps a hidden input, so GET forms submit it as before.
import { type KeyboardEvent, useEffect, useId, useRef, useState } from "react";

import { Icon } from "./Icon.tsx";

export interface SelectOption {
  value: string;
  label: string;
  /** Hero portrait or similar, shown before the label. */
  icon?: string;
}

const TYPEAHEAD_MS = 600;

export function Select({
  label,
  ariaLabel,
  options,
  value,
  defaultValue = "",
  onChange,
  name,
}: {
  /** Visible caption inside the field. */
  label: string;
  /** Accessible name; defaults to the label. */
  ariaLabel?: string;
  options: SelectOption[];
  /** Controlled value; omit to let the component keep its own. */
  value?: string;
  defaultValue?: string;
  onChange?: (value: string) => void;
  /** Form field name: a hidden input carries the current value, if any. */
  name?: string;
}) {
  const id = useId();
  const [own, setOwn] = useState(defaultValue);
  const current = value ?? own;
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const [alignRight, setAlignRight] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const typed = useRef({ text: "", at: 0 });

  const selectedIndex = Math.max(
    0,
    options.findIndex((o) => o.value === current),
  );
  const selected = options[selectedIndex];

  // Close on a click anywhere else.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open]);

  // Keep the active option in view and the popup inside the window.
  useEffect(() => {
    if (!open) return;
    list.current?.querySelector(`[data-index="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [open, active]);

  function show() {
    setActive(selectedIndex);
    const box = root.current?.getBoundingClientRect();
    setAlignRight(!!box && box.left + 260 > window.innerWidth);
    setOpen(true);
  }

  function choose(i: number) {
    const o = options[i];
    if (!o) return;
    if (value === undefined) setOwn(o.value);
    if (o.value !== current) onChange?.(o.value);
    setOpen(false);
  }

  function typeahead(ch: string) {
    const now = Date.now();
    const t = typed.current;
    t.text = now - t.at < TYPEAHEAD_MS ? t.text + ch.toLowerCase() : ch.toLowerCase();
    t.at = now;
    const from = open ? active : selectedIndex;
    const order = [...options.keys()].map((k) => (from + 1 + k) % options.length);
    // A repeated single letter cycles; a longer prefix starts from the current one.
    const start = t.text.length > 1 ? [from, ...order] : order;
    const hit = start.find((i) => options[i]!.label.toLowerCase().startsWith(t.text));
    if (hit === undefined) return;
    if (open) setActive(hit);
    else choose(hit);
  }

  function onKeyDown(e: KeyboardEvent) {
    const last = options.length - 1;
    switch (e.key) {
      case "ArrowDown":
      case "ArrowUp": {
        e.preventDefault();
        if (!open) return show();
        const step = e.key === "ArrowDown" ? 1 : -1;
        setActive((a) => Math.min(last, Math.max(0, a + step)));
        return;
      }
      case "Home":
      case "End":
        if (!open) return;
        e.preventDefault();
        setActive(e.key === "Home" ? 0 : last);
        return;
      case "Enter":
      case " ":
        e.preventDefault();
        if (open) choose(active);
        else show();
        return;
      case "Escape":
        if (open) {
          e.preventDefault();
          setOpen(false);
        }
        return;
      case "Tab":
        setOpen(false);
        return;
      default:
        if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) typeahead(e.key);
    }
  }

  const listId = `${id}-list`;
  return (
    <div className={`field select${open ? " open" : ""}`} ref={root}>
      <span>{label}</span>
      {/* "Any" (empty) is not submitted: the URL keeps only real filters. */}
      {name !== undefined && current !== "" && <input type="hidden" name={name} value={current} />}
      <button
        type="button"
        role="combobox"
        aria-label={ariaLabel ?? label}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listId}
        aria-activedescendant={open ? `${id}-${active}` : undefined}
        className="select-button"
        onClick={() => (open ? setOpen(false) : show())}
        onKeyDown={onKeyDown}
      >
        {selected?.icon && <Icon src={selected.icon} size={18} />}
        <span className="select-value">{selected?.label ?? ""}</span>
        <svg className="chevron" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" aria-hidden="true">
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>
      <ul
        id={listId}
        ref={list}
        role="listbox"
        aria-label={ariaLabel ?? label}
        className={`select-list${alignRight ? " right" : ""}`}
        hidden={!open}
      >
        {options.map((o, i) => (
          <li
            key={o.value}
            id={`${id}-${i}`}
            data-index={i}
            role="option"
            aria-selected={i === selectedIndex}
            className={i === active ? "active" : undefined}
            onPointerEnter={() => setActive(i)}
            // Keep focus on the button: the list is not focusable.
            onPointerDown={(e) => e.preventDefault()}
            onClick={() => choose(i)}
          >
            {o.icon && <Icon src={o.icon} size={20} />}
            <span>{o.label}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
