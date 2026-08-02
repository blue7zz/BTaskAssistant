/*
 * Model and effort switchers copied 1:1 from Reasonix (ModelSwitcher.tsx /
 * EffortSwitcher.tsx), adapted only to the BTask workbench bridge
 * (piSettings + updatePISettings instead of backend model catalogs).
 * Reasonix is MIT licensed; see THIRD_PARTY_NOTICES.md.
 */

import {
  Brain,
  Check,
  ChevronsUpDown,
  Gauge,
  Search,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import type { PIThinkingEffort } from "../domain/engine";

export interface PIModelInfo {
  provider: string;
  model: string;
  ref: string;
  current?: boolean;
}

interface ReasonixModelSwitcherProps {
  label: string;
  models: PIModelInfo[];
  disabled?: boolean;
  onPick(ref: string): void | Promise<void>;
}

export function ReasonixModelSwitcher({
  label,
  models,
  disabled = false,
  onPick,
}: ReasonixModelSwitcherProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const triggerRef = useRef<HTMLButtonElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) window.requestAnimationFrame(() => inputRef.current?.focus());
  }, [open]);

  const keyword = query.trim().toLowerCase();
  const filtered = useMemo(
    () =>
      keyword
        ? models.filter(
            (m) =>
              m.model.toLowerCase().includes(keyword) ||
              m.provider.toLowerCase().includes(keyword),
          )
        : models,
    [keyword, models],
  );
  const groups = useMemo(() => {
    const byProvider = new Map<string, PIModelInfo[]>();
    for (const model of filtered) {
      const list = byProvider.get(model.provider) ?? [];
      list.push(model);
      byProvider.set(model.provider, list);
    }
    return Array.from(byProvider.entries());
  }, [filtered]);
  const currentProvider = useMemo(
    () => models.find((m) => m.current)?.provider ?? null,
    [models],
  );
  const triggerLabel = currentProvider ? `${label} · ${currentProvider}` : label;

  const pick = (model: PIModelInfo) => {
    setOpen(false);
    void onPick(model.ref);
  };

  return (
    <div className="modelsw">
      <button
        ref={triggerRef}
        type="button"
        className="modelsw__trigger"
        aria-label={triggerLabel}
        aria-expanded={open}
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
      >
        <Brain size={14} className="modelsw__kind" />
        <span className="modelsw__label">{label}</span>
        <ChevronsUpDown size={11} />
      </button>
      {open && (
        <div className="modelsw__menu modelsw__menu--portal">
          <div role="listbox">
            <div className="modelsw__search" role="presentation">
              <Search size={13} />
              <input
                ref={inputRef}
                type="text"
                className="modelsw__search-input"
                placeholder="搜索模型…"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Escape") setOpen(false);
                  if (e.key === "Enter" && filtered.length === 1) pick(filtered[0]);
                }}
              />
            </div>
            {models.length === 0 && <div className="modelsw__empty">暂无可用模型</div>}
            {models.length > 0 && filtered.length === 0 && query && (
              <div className="modelsw__empty">没有匹配的模型</div>
            )}
            {groups.map(([provider, items]) => (
              <div key={provider} role="group" aria-label={provider} className="modelsw__group">
                <div className="modelsw__group-label" role="presentation">
                  <Brain size={11} />
                  {provider}
                </div>
                {items.map((m) => (
                  <button
                    key={m.ref}
                    type="button"
                    role="option"
                    aria-selected={m.current}
                    className={`modelsw__item ${m.current ? "modelsw__item--current" : ""}`}
                    onClick={() => pick(m)}
                  >
                    <span className="modelsw__copy">
                      <span className="modelsw__model">{m.model}</span>
                    </span>
                    {m.current && <Check size={13} className="modelsw__check" />}
                  </button>
                ))}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

interface ReasonixEffortSwitcherProps {
  effort: string;
  levels: PIThinkingEffort[];
  disabled?: boolean;
  onPick(level: string): void;
}

export function ReasonixEffortSwitcher({
  effort,
  levels,
  disabled = false,
  onPick,
}: ReasonixEffortSwitcherProps) {
  const [open, setOpen] = useState(false);
  const current = effort || "xhigh";

  return (
    <div className="modelsw effortsw">
      <button
        type="button"
        className={`modelsw__trigger effortsw__trigger ${current !== "auto" ? "effortsw__trigger--explicit" : ""}`}
        disabled={disabled}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <Gauge size={14} className="modelsw__kind" />
        <span className="modelsw__label">{current}</span>
        <ChevronsUpDown size={11} />
      </button>
      {open && (
        <div className="modelsw__menu modelsw__menu--portal effortsw__menu">
          <div role="listbox">
            {levels.map((level) => (
              <button
                key={level}
                type="button"
                role="option"
                aria-selected={level === current}
                className={`modelsw__item ${level === current ? "modelsw__item--current" : ""}`}
                onClick={() => {
                  setOpen(false);
                  if (level !== current) onPick(level);
                }}
              >
                <span className="modelsw__model">{level}</span>
                {level === current && <Check size={13} className="modelsw__check" />}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
