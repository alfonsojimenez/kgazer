import { useMemo, useRef, useState, useCallback, useEffect } from "react";
import type { TimelinePoint } from "@/lib/api";

interface DiffTimelineProps {
  points: TimelinePoint[];
  onSelectVersion?: (offset: number) => void;
}

interface Bucket {
  x: number;
  width: number;
  height: number;
  colour: string;
  count: number;
  maxBodySize: number;
  representative: TimelinePoint;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const kb = bytes / 1024;
  if (kb < 1024) return `${kb.toFixed(1)} KB`;
  const mb = kb / 1024;
  return `${mb.toFixed(1)} MB`;
}

function formatTooltipDate(ts: string): string {
  const d = new Date(ts);
  const day = d.getDate();
  const month = d.toLocaleString("en-GB", { month: "short" });
  const year = d.getFullYear();
  const hours = String(d.getHours()).padStart(2, "0");
  const mins = String(d.getMinutes()).padStart(2, "0");
  return `${day} ${month} ${year}, ${hours}:${mins}`;
}

function formatAxisLabel(ts: number, spanMs: number): string {
  const d = new Date(ts);
  const day = d.getDate();
  const month = d.toLocaleString("en-GB", { month: "short" });
  const hours = String(d.getHours()).padStart(2, "0");
  const mins = String(d.getMinutes()).padStart(2, "0");

  const ONE_DAY = 86_400_000;
  const ONE_YEAR = 365.25 * ONE_DAY;

  if (spanMs < ONE_DAY) return `${hours}:${mins}`;
  if (spanMs < ONE_YEAR) return `${day} ${month}`;
  return `${month} ${d.getFullYear()}`;
}

/**
 * HSL interpolation across three stops: blue-500 (217°) → violet-500 (258°) → amber-500 (38°).
 * The violet→amber transition wraps around the hue circle (258° → 360° → 38°).
 */
function sizeToColour(bodySize: number, median: number): string {
  const ratio = median > 0 ? bodySize / median : 0;

  if (ratio <= 1) {
    const t = Math.max(0, Math.min(1, ratio));
    const h = 217 + (258 - 217) * t;
    const s = 91 + (90 - 91) * t;
    const l = 60 + (66 - 60) * t;
    return `hsl(${h}, ${s}%, ${l}%)`;
  }

  const t = Math.max(0, Math.min(1, (ratio - 1) / 1));
  const h = 258 + (360 + 38 - 258) * t;
  const s = 90 + (92 - 90) * t;
  const l = 66 + (50 - 66) * t;
  return `hsl(${h > 360 ? h - 360 : h}, ${s}%, ${l}%)`;
}

function computeMedian(values: number[]): number {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 !== 0
    ? sorted[mid]
    : (sorted[mid - 1] + sorted[mid]) / 2;
}

const CHART_HEIGHT = 64;
const AXIS_HEIGHT = 20;
const PADDING_X = 0;
const MIN_BAR_HEIGHT = 2;
const MAX_BUCKET_COUNT = 200;
const CONTAINER_PADDING = 32;
const AXIS_LABEL_OFFSET = 16;

export function DiffTimeline({ points, onSelectVersion }: DiffTimelineProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(0);
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null);
  const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 });

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width);
      }
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const sorted = useMemo(
    () => [...points].sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()),
    [points],
  );

  const median = useMemo(
    () => computeMedian(sorted.map((p) => p.body_size)),
    [sorted],
  );

  const buckets = useMemo((): Bucket[] => {
    if (sorted.length === 0 || containerWidth === 0) return [];

    const times = sorted.map((p) => new Date(p.timestamp).getTime());
    const minTime = times[0];
    const maxTime = times[times.length - 1];
    const span = maxTime - minTime;

    const maxBodySize = Math.max(...sorted.map((p) => p.body_size), 1);
    const chartW = containerWidth - PADDING_X * 2;

    if (span === 0) {
      const height = Math.max(
        MIN_BAR_HEIGHT,
        (sorted[0].body_size / maxBodySize) * CHART_HEIGHT,
      );
      return [
        {
          x: chartW / 2 - 2,
          width: 4,
          height,
          colour: sizeToColour(sorted[0].body_size, median),
          count: sorted.length,
          maxBodySize: sorted[0].body_size,
          representative: sorted[0],
        },
      ];
    }

    const needsBucketing = sorted.length > MAX_BUCKET_COUNT;

    if (!needsBucketing) {
      const slotW = chartW / sorted.length;
      const barW = Math.max(2, Math.min(8, slotW - 1.5));

      return sorted.map((p, i) => {
        const x = i * slotW + (slotW - barW) / 2;
        const height = Math.max(
          MIN_BAR_HEIGHT,
          (p.body_size / maxBodySize) * CHART_HEIGHT,
        );
        return {
          x,
          width: barW,
          height,
          colour: sizeToColour(p.body_size, median),
          count: 1,
          maxBodySize: p.body_size,
          representative: p,
        };
      });
    }

    const bucketCount = Math.min(sorted.length, MAX_BUCKET_COUNT);
    const bucketWidth = span / bucketCount;
    const slotW = chartW / bucketCount;
    const barW = Math.max(2, Math.min(8, slotW - 1.5));
    const barGap = slotW - barW;

    const result: Bucket[] = [];
    let pIdx = 0;

    for (let b = 0; b < bucketCount; b++) {
      const bucketEnd = b === bucketCount - 1 ? maxTime + 1 : minTime + (b + 1) * bucketWidth;

      let count = 0;
      let maxBS = 0;
      let rep: TimelinePoint | null = null;

      while (pIdx < sorted.length && times[pIdx] < bucketEnd) {
        count++;
        if (sorted[pIdx].body_size > maxBS) {
          maxBS = sorted[pIdx].body_size;
          rep = sorted[pIdx];
        }
        pIdx++;
      }

      if (count === 0 || !rep) continue;

      const x = b * (barW + barGap);
      const height = Math.max(
        MIN_BAR_HEIGHT,
        (maxBS / maxBodySize) * CHART_HEIGHT,
      );

      result.push({
        x,
        width: barW,
        height,
        colour: sizeToColour(maxBS, median),
        count,
        maxBodySize: maxBS,
        representative: rep,
      });
    }
    return result;
  }, [sorted, containerWidth, median]);

  const axisLabels = useMemo(() => {
    if (sorted.length < 2) return [];
    const times = sorted.map((p) => new Date(p.timestamp).getTime());
    const minTime = times[0];
    const maxTime = times[times.length - 1];
    const span = maxTime - minTime;
    if (span === 0) return [];

    const chartW = containerWidth - PADDING_X * 2;
    const count = chartW > 500 ? 5 : chartW > 300 ? 4 : 3;
    const labels: { x: number; text: string }[] = [];

    for (let i = 0; i < count; i++) {
      const frac = i / (count - 1);
      const ts = minTime + span * frac;
      labels.push({
        x: frac * chartW,
        text: formatAxisLabel(ts, span),
      });
    }
    return labels;
  }, [sorted, containerWidth]);

  const handleBarHover = useCallback(
    (idx: number, event: React.MouseEvent<SVGRectElement>) => {
      setHoveredIdx(idx);
      const rect = event.currentTarget.getBoundingClientRect();
      const container = containerRef.current?.getBoundingClientRect();
      if (!container) return;
      setTooltipPos({
        x: rect.left - container.left + rect.width / 2,
        y: rect.top - container.top,
      });
    },
    [],
  );

  const handleBarClick = useCallback(
    (bucket: Bucket) => {
      onSelectVersion?.(bucket.representative.offset);
    },
    [onSelectVersion],
  );

  if (points.length <= 1) {
    return (
      <div className="rounded-xl border border-border bg-card px-6 py-8 text-center">
        <p className="text-sm text-muted-foreground">
          First version — no change history yet
        </p>
      </div>
    );
  }

  const isBucketed = sorted.length > MAX_BUCKET_COUNT;
  const hovered = hoveredIdx !== null ? buckets[hoveredIdx] : null;
  const svgWidth = Math.max(0, containerWidth - CONTAINER_PADDING);

  return (
    <div
      ref={containerRef}
      className="relative rounded-xl border border-border bg-card overflow-x-clip"
    >
      <div className="flex items-center justify-between px-4 pt-2 pb-0.5">
        <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          Change timeline
        </span>
        {isBucketed && (
          <span className="text-[11px] tabular-nums text-muted-foreground">
            {sorted.length.toLocaleString()} versions
          </span>
        )}
      </div>

      <div className="px-4 pb-0.5">
        <svg
          width={svgWidth}
          height={CHART_HEIGHT}
          className="block"
          style={{ overflow: "visible" }}
        >
          <line
            x1={0}
            y1={CHART_HEIGHT}
            x2={svgWidth}
            y2={CHART_HEIGHT}
            stroke="currentColor"
            className="text-border"
            strokeWidth={1}
          />

          {buckets.map((bucket, i) => (
            <rect
              key={i}
              x={bucket.x}
              y={CHART_HEIGHT - bucket.height}
              width={bucket.width}
              height={bucket.height}
              rx={Math.min(bucket.width / 2, 2)}
              fill={bucket.colour}
              opacity={hoveredIdx === null || hoveredIdx === i ? 0.85 : 0.35}
              className="transition-opacity duration-150 cursor-pointer"
              onMouseEnter={(e) => handleBarHover(i, e)}
              onMouseLeave={() => setHoveredIdx(null)}
              onClick={() => handleBarClick(bucket)}
            />
          ))}
        </svg>
      </div>

      <div
        className="relative px-4 pb-2"
        style={{ height: AXIS_HEIGHT }}
      >
        {axisLabels.map((label, i) => (
          <span
            key={i}
            className="absolute text-[11px] tabular-nums text-muted-foreground select-none"
            style={{
              left: label.x + AXIS_LABEL_OFFSET,
              top: 0,
              transform:
                i === 0
                  ? "translateX(0)"
                  : i === axisLabels.length - 1
                    ? "translateX(-100%)"
                    : "translateX(-50%)",
            }}
          >
            {label.text}
          </span>
        ))}
      </div>

      {hovered && hoveredIdx !== null && (
        <div
          className="pointer-events-none absolute z-50 rounded-lg border border-border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-xl"
          style={{
            left: tooltipPos.x,
            top: tooltipPos.y - 8,
            transform: "translate(-50%, -100%)",
            whiteSpace: "nowrap",
          }}
        >
          <div className="font-medium">
            {formatTooltipDate(hovered.representative.timestamp)}
          </div>
          <div className="mt-0.5 text-muted-foreground">
            Offset{" "}
            <span className="font-mono tabular-nums">
              {hovered.representative.offset.toLocaleString()}
            </span>
            {" · "}Partition{" "}
            <span className="font-mono tabular-nums">
              {hovered.representative.partition}
            </span>
          </div>
          <div className="mt-0.5 text-muted-foreground">
            Body:{" "}
            <span className="font-mono tabular-nums">
              {formatBytes(hovered.maxBodySize)}
            </span>
            {hovered.count > 1 && (
              <span className="ml-1">
                ({hovered.count} versions)
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
