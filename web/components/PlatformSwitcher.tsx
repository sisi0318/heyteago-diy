"use client";

// 平台切换：各平台的画布规格、底色与 token 相互独立，切换时由页面整体重置。
import { PLATFORM_LIST, type PlatformId } from "@/lib/platforms";

interface Props {
  value: PlatformId;
  disabled: boolean;
  onChange(id: PlatformId): void;
}

export function PlatformSwitcher({ value, disabled, onChange }: Props) {
  return (
    <div role="tablist" aria-label="平台" className="flex gap-1 rounded-lg bg-neutral-200/70 p-1">
      {PLATFORM_LIST.map((p) => (
        <button
          key={p.id}
          type="button"
          role="tab"
          aria-selected={p.id === value}
          disabled={disabled}
          onClick={() => onChange(p.id)}
          className={`rounded-md px-3 py-1 text-xs disabled:opacity-50 ${
            p.id === value
              ? "bg-white font-medium text-neutral-900 shadow-sm"
              : "text-neutral-600 hover:text-neutral-900"
          }`}
        >
          {p.name}
        </button>
      ))}
    </div>
  );
}
