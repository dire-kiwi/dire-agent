import { formatContext, formatTokens } from "../../lib/display";
import type { Usage } from "../../lib/protocol";

export function UsageSummary({ usage, contextWindow }: { usage: Usage; contextWindow: number }) {
  return (
    <div className="usage-summary grid min-w-0 grid-cols-4 overflow-hidden rounded-lg border border-white/[0.08] bg-white/[0.025] xl:grid-cols-[repeat(4,minmax(58px,auto))_minmax(180px,1fr)]" aria-label="Token usage">
      <Counter label="Input" value={usage.input_tokens} />
      <Counter label="Output" value={usage.output_tokens} />
      <Counter label="Cache read" value={usage.cache_read_tokens} />
      <Counter
        label="Cache write"
        value={usage.cache_write_tokens}
        hint="Provider-reported value. The Codex subscription stream currently omits this counter even when a later cache read proves the prefix was stored."
      />
      <div className="context-summary col-span-4 grid min-w-0 content-center gap-1.5 border-t border-white/[0.08] px-2.5 py-1.5 xl:col-span-1 xl:border-t-0">
        <div className="context-copy">
          <small>Context</small>
          <strong>{formatContext(usage.context_tokens, contextWindow)}</strong>
        </div>
        {contextWindow > 0 && (
          <progress
            aria-label="Context used"
            value={Math.min(usage.context_tokens, contextWindow)}
            max={contextWindow}
          />
        )}
      </div>
    </div>
  );
}

function Counter({ label, value, hint }: { label: string; value: number; hint?: string }) {
  return (
    <span className="usage-counter" title={hint}>
      <small>{label}</small>
      <strong>{formatTokens(value)}</strong>
    </span>
  );
}
