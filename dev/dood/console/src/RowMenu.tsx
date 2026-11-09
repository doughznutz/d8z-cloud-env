import { useEffect, useRef, useState } from "react";

/** One action in a {@link RowMenu}: either a click handler or a plain link. */
export interface RowMenuItem {
  label: string;
  onClick?: () => void;
  href?: string;
  disabled?: boolean;
  danger?: boolean;
}

/** Props for {@link RowMenu}. */
export interface RowMenuProps {
  items: RowMenuItem[];
  title?: string;
  /** Trigger glyph; default "⋮". The Services/Workspaces section headers pass "+". */
  icon?: string;
  /** Extra class(es) on the trigger's wrapper, e.g. "section-action" to float it next to a heading. */
  className?: string;
}

/**
 * A small dropdown (default trigger "⋮") for a sidebar row's or section
 * header's actions (Settings, Start, View, Stop, Edit, Remove, ...). Closes
 * on an outside click, Escape, or picking an item. Caller positions it
 * (typically inside a row's existing `.row-actions` wrapper, or a section
 * heading).
 */
export function RowMenu({ items, title = "Actions", icon = "⋮", className }: RowMenuProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLSpanElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <span className={`row-menu-wrap${className ? ` ${className}` : ""}`} ref={ref}>
      <button
        type="button"
        className="icon-btn row-menu-trigger"
        title={title}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          setOpen((o) => !o);
        }}
      >
        {icon}
      </button>
      {open && (
        <ul className="row-menu">
          {items.map((it) => (
            <li key={it.label}>
              {it.href ? (
                <a
                  href={it.disabled ? undefined : it.href}
                  className={it.danger ? "danger" : ""}
                  aria-disabled={it.disabled}
                  onClick={(e) => {
                    if (it.disabled) {
                      e.preventDefault();
                      return;
                    }
                    setOpen(false);
                  }}
                >
                  {it.label}
                </a>
              ) : (
                <button
                  type="button"
                  className={it.danger ? "danger" : ""}
                  disabled={it.disabled}
                  onClick={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    setOpen(false);
                    it.onClick?.();
                  }}
                >
                  {it.label}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </span>
  );
}
