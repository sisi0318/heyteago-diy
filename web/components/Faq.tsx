// 常见问题（静态内容，按平台取自平台注册表）。
import type { FaqItem } from "@/lib/platforms";

export function Faq({ items }: { items: FaqItem[] }) {
  return (
    <section className="rounded-xl border border-neutral-200 bg-white p-4 shadow-sm">
      <h2 className="mb-3 text-sm font-semibold text-neutral-800">常见问题</h2>
      <div className="space-y-3 text-xs">
        {items.map((item) => (
          <div key={item.q}>
            <p className="font-medium text-neutral-800">{item.q}</p>
            {item.a.map((line) => (
              <p key={line} className="mt-0.5 leading-relaxed text-neutral-500">
                {line}
              </p>
            ))}
          </div>
        ))}
      </div>
    </section>
  );
}
