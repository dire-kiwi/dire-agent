import { Check, ChevronDown } from "lucide-react";
import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";

export interface CompactSelectOption {
  value: string;
  label: string;
}

interface CompactSelectProps {
  ariaLabel: string;
  className?: string;
  disabled?: boolean;
  icon: ReactNode;
  onChange: (value: string) => void;
  options: readonly CompactSelectOption[];
  title?: string;
  value: string;
}

export function CompactSelect(props: CompactSelectProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const reactID = useId().replaceAll(":", "");
  const listboxID = `compact-select-${reactID}`;
  const selectedIndex = Math.max(0, props.options.findIndex((option) => option.value === props.value));
  const selected = props.options[selectedIndex];
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(selectedIndex);

  useEffect(() => {
    if (!open) return;
    const closeOutside = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", closeOutside);
    return () => document.removeEventListener("pointerdown", closeOutside);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    optionRefs.current[activeIndex]?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, open]);

  useEffect(() => {
    if (props.disabled) setOpen(false);
  }, [props.disabled]);

  const openMenu = () => {
    if (props.disabled || props.options.length === 0) return;
    setActiveIndex(selectedIndex);
    setOpen(true);
  };

  const choose = (index: number) => {
    const option = props.options[index];
    if (!option) return;
    props.onChange(option.value);
    setOpen(false);
    triggerRef.current?.focus();
  };

  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (props.disabled || props.options.length === 0) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (!open) {
        openMenu();
        return;
      }
      const direction = event.key === "ArrowDown" ? 1 : -1;
      setActiveIndex((current) => (current + direction + props.options.length) % props.options.length);
      return;
    }
    if (event.key === "Home" || event.key === "End") {
      event.preventDefault();
      if (!open) openMenu();
      setActiveIndex(event.key === "Home" ? 0 : props.options.length - 1);
      return;
    }
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      if (open) choose(activeIndex);
      else openMenu();
      return;
    }
    if (event.key === "Escape" && open) {
      event.preventDefault();
      setOpen(false);
      return;
    }
    if (event.key === "Tab") setOpen(false);
  };

  return (
    <div
      ref={rootRef}
      className={`mode-select${open ? " open" : ""}${props.className ? ` ${props.className}` : ""}`}
      title={props.title}
    >
      <button
        ref={triggerRef}
        type="button"
        className="compact-select-trigger"
        role="combobox"
        aria-label={props.ariaLabel}
        aria-expanded={open}
        aria-controls={listboxID}
        aria-haspopup="listbox"
        aria-activedescendant={open ? `${listboxID}-option-${activeIndex}` : undefined}
        disabled={props.disabled}
        onClick={() => open ? setOpen(false) : openMenu()}
        onKeyDown={onKeyDown}
      >
        <span className="compact-select-icon" aria-hidden="true">{props.icon}</span>
        <span className="compact-select-value">{selected?.label || props.value}</span>
        <ChevronDown className="compact-select-chevron" size={13} aria-hidden="true" />
      </button>
      {open && (
        <div id={listboxID} className="compact-select-menu" role="listbox" aria-label={`${props.ariaLabel} options`}>
          {props.options.map((option, index) => {
            const isSelected = option.value === props.value;
            const isActive = index === activeIndex;
            return (
              <button
                ref={(element) => { optionRefs.current[index] = element; }}
                key={option.value}
                id={`${listboxID}-option-${index}`}
                type="button"
                className={`compact-select-option${isActive ? " active" : ""}`}
                role="option"
                aria-selected={isSelected}
                tabIndex={-1}
                onMouseDown={(event) => event.preventDefault()}
                onMouseMove={() => setActiveIndex(index)}
                onClick={() => choose(index)}
              >
                <span>{option.label}</span>
                {isSelected && <Check size={13} aria-hidden="true" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
