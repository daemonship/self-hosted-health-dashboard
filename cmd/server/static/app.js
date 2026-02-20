import { h, render } from 'preact';
import { useState, useEffect, useRef, useCallback } from 'preact/hooks';
import htm from 'htm';

const html = htm.bind(h);

// ─── Constants ───────────────────────────────────────────────────────────────

const REFRESH_MS = 30_000;

// ─── API helpers ─────────────────────────────────────────────────────────────

async function apiFetch(path) {
  const res = await fetch(path, { credentials: 'include' });
  if (res.status === 401) {
    window.location.href = '/login';
    return null;
  }
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

// ─── Formatting helpers ───────────────────────────────────────────────────────

function fmtBytes(bytes) {
  if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB';
  if (bytes >= 1048576)    return (bytes / 1048576).toFixed(0)    + ' MB';
  if (bytes >= 1024)       return (bytes / 1024).toFixed(0)       + ' KB';
  return bytes + ' B';
}

function gaugeStroke(pct) {
  if (pct >= 90) return '#f87171';
  if (pct >= 70) return '#f59e0b';
  return '#6366f1';
}

// ─── StatusPill ──────────────────────────────────────────────────────────────

const STATUS_STYLES = {
  up:      { label: 'UP',      color: '#22c55e', bg: 'rgba(34,197,94,0.10)',  border: 'rgba(34,197,94,0.25)'  },
  down:    { label: 'DOWN',    color: '#f87171', bg: 'rgba(248,113,113,0.10)', border: 'rgba(248,113,113,0.25)' },
  unknown: { label: 'UNKNOWN', color: '#64748b', bg: 'rgba(100,116,139,0.10)', border: 'rgba(100,116,139,0.25)' },
};

function StatusPill({ state }) {
  const s = STATUS_STYLES[state] || STATUS_STYLES.unknown;
  return html`<span class="status-pill" style="background:${s.bg};color:${s.color};border:1px solid ${s.border}">${s.label}</span>`;
}

// ─── MonitorCard ─────────────────────────────────────────────────────────────

function MonitorCard({ m }) {
  const latency = m.last_response_ms != null ? `${m.last_response_ms} ms` : '—';
  const uptime  = m.uptime_24h      != null ? `${m.uptime_24h.toFixed(1)}%` : '—';
  return html`
    <div class="monitor-card">
      <div class="monitor-top">
        <${StatusPill} state=${m.state} />
        <span class="monitor-name">${m.name}</span>
      </div>
      <div class="monitor-url">${m.url}</div>
      <div class="monitor-stats">
        <span class="stat"><span class="stat-label">Latency</span>${latency}</span>
        <span class="stat"><span class="stat-label">24 h uptime</span>${uptime}</span>
      </div>
    </div>`;
}

// ─── MonitorsSection ─────────────────────────────────────────────────────────

function MonitorsSection({ monitors, loading }) {
  return html`
    <section class="section">
      <h2 class="section-title">Uptime Monitors</h2>
      ${loading
        ? html`<p class="muted">Loading…</p>`
        : monitors.length === 0
          ? html`<p class="muted">No monitors configured. Add one via <code>POST /api/monitors</code>.</p>`
          : html`<div class="monitors-grid">${monitors.map(m => html`<${MonitorCard} key=${m.id} m=${m} />`)}</div>`}
    </section>`;
}

// ─── Gauge (SVG half-circle) ─────────────────────────────────────────────────
//
// Arc path from (10,50) to (90,50) sweeping clockwise — radius 40, centre (50,50).
// Path length = π × 40 ≈ 125.66.  stroke-dasharray fill = pct/100 × 125.66.

const ARC_LEN = Math.PI * 40; // 125.66

function Gauge({ label, pct, subtitle }) {
  const safeP = Math.min(Math.max(pct ?? 0, 0), 100);
  const fill  = (safeP / 100) * ARC_LEN;
  const color = gaugeStroke(safeP);
  return html`
    <div class="gauge">
      <svg viewBox="0 0 100 56" width="110" height="62" aria-label="${label} ${Math.round(safeP)}%">
        <path d="M10,50 A40,40 0 0,1 90,50" fill="none" stroke="#1e293b"   stroke-width="9" stroke-linecap="round"/>
        <path d="M10,50 A40,40 0 0,1 90,50" fill="none" stroke="${color}"  stroke-width="9" stroke-linecap="round"
              stroke-dasharray="${fill.toFixed(2)} 999"/>
        <text x="50" y="50" text-anchor="middle" dominant-baseline="auto"
              fill="#f1f5f9" font-size="14" font-weight="700" font-family="-apple-system,sans-serif">
          ${Math.round(safeP)}%
        </text>
      </svg>
      <div class="gauge-label">${label}</div>
      ${subtitle ? html`<div class="gauge-sub">${subtitle}</div>` : null}
    </div>`;
}

// ─── MetricsChart (uPlot) ────────────────────────────────────────────────────

function MetricsChart({ series }) {
  const containerRef = useRef(null);
  const chartRef     = useRef(null);

  // Build / rebuild chart whenever series data changes.
  useEffect(() => {
    if (!containerRef.current || !series || series.length === 0) return;

    if (chartRef.current) { chartRef.current.destroy(); chartRef.current = null; }

    const timestamps = series.map(d => d.ts);
    const cpu        = series.map(d => d.cpu_percent);
    const mem        = series.map(d => d.mem_total > 0 ? (d.mem_used / d.mem_total) * 100 : null);

    const w = containerRef.current.clientWidth || 700;

    const opts = {
      width:  w,
      height: 180,
      series: [
        {},
        { label: 'CPU %',    stroke: '#6366f1', width: 1.5, fill: 'rgba(99,102,241,0.07)'  },
        { label: 'Memory %', stroke: '#22c55e', width: 1.5, fill: 'rgba(34,197,94,0.07)'   },
      ],
      axes: [
        { stroke: '#475569', grid: { stroke: '#1e293b' }, ticks: { stroke: '#1e293b' } },
        {
          stroke: '#475569',
          grid:   { stroke: '#1e293b' },
          ticks:  { stroke: '#1e293b' },
          values: (_u, vals) => vals.map(v => v != null ? v.toFixed(0) + '%' : ''),
          size:   46,
        },
      ],
      scales: { y: { auto: false, range: [0, 100] } },
      cursor: { show: true },
    };

    chartRef.current = new uPlot(opts, [timestamps, cpu, mem], containerRef.current);

    return () => { if (chartRef.current) { chartRef.current.destroy(); chartRef.current = null; } };
  }, [series]);

  // Resize chart when container width changes.
  useEffect(() => {
    if (!containerRef.current) return;
    const obs = new ResizeObserver(() => {
      if (chartRef.current && containerRef.current) {
        chartRef.current.setSize({ width: containerRef.current.clientWidth, height: 180 });
      }
    });
    obs.observe(containerRef.current);
    return () => obs.disconnect();
  }, []);

  if (!series || series.length === 0) {
    return html`<p class="muted">No metrics history yet — ensure the agent is running.</p>`;
  }
  return html`<div ref=${containerRef}></div>`;
}

// ─── MetricsSection ──────────────────────────────────────────────────────────

function MetricsSection({ data, loading }) {
  if (loading) {
    return html`<section class="section"><h2 class="section-title">System Metrics</h2><p class="muted">Loading…</p></section>`;
  }

  const latest = data?.latest;
  const series = data?.series ?? [];
  const disks  = latest?.disks ?? [];

  const cpuPct = latest?.cpu_percent ?? 0;
  const memPct = latest ? (latest.mem_used / latest.mem_total) * 100 : 0;

  return html`
    <section class="section">
      <h2 class="section-title">System Metrics</h2>
      ${!latest
        ? html`<p class="muted">No metrics yet — ensure the agent is running and pointed at this server.</p>`
        : html`
          <div class="gauges-row">
            <${Gauge} label="CPU" pct=${cpuPct} subtitle="utilization" />
            <${Gauge} label="Memory" pct=${memPct}
              subtitle="${fmtBytes(latest.mem_used)} / ${fmtBytes(latest.mem_total)}" />
            ${disks.map((d, i) => {
              const dp = d.total > 0 ? (d.used / d.total) * 100 : 0;
              return html`<${Gauge} key=${i} label=${d.mount} pct=${dp}
                subtitle="${fmtBytes(d.used)} / ${fmtBytes(d.total)}" />`;
            })}
          </div>
          <div class="chart-wrap">
            <${MetricsChart} series=${series} />
          </div>`}
    </section>`;
}

// ─── EventsSection ───────────────────────────────────────────────────────────

function EventsSection({ events, loading }) {
  return html`
    <section class="section">
      <h2 class="section-title">Business Events</h2>
      ${loading
        ? html`<p class="muted">Loading…</p>`
        : events.length === 0
          ? html`<p class="muted">No events recorded yet. Send one via <code>POST /api/events</code>.</p>`
          : html`
            <div class="events-table">
              <div class="events-header">
                <span>Event</span>
                <span>Today</span>
                <span>Last 7 Days</span>
              </div>
              ${events.map(e => html`
                <div key=${e.event_name} class="events-row">
                  <span class="event-name">${e.event_name}</span>
                  <span class="event-today">${e.today.toLocaleString()}</span>
                  <span class="event-7d">${e.last_7_days.toLocaleString()}</span>
                </div>`)}
            </div>`}
    </section>`;
}

// ─── App ─────────────────────────────────────────────────────────────────────

function App() {
  const [monitors, setMonitors] = useState([]);
  const [metrics,  setMetrics]  = useState(null);
  const [events,   setEvents]   = useState([]);
  const [loading,  setLoading]  = useState(true);
  const [updated,  setUpdated]  = useState(null);
  const [error,    setError]    = useState(null);

  const fetchAll = useCallback(async () => {
    try {
      const [mon, met, evt] = await Promise.all([
        apiFetch('/api/dashboard/monitors'),
        apiFetch('/api/dashboard/metrics'),
        apiFetch('/api/dashboard/events'),
      ]);
      // null means a 401 redirect is in progress — bail out silently.
      if (mon === null || met === null || evt === null) return;
      setMonitors(mon ?? []);
      setMetrics(met);
      setEvents(evt ?? []);
      setUpdated(new Date());
      setError(null);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAll();
    const id = setInterval(fetchAll, REFRESH_MS);
    return () => clearInterval(id);
  }, [fetchAll]);

  return html`
    <div class="layout">
      <header class="topbar">
        <span class="topbar-title">Health Dashboard</span>
        <div class="topbar-right">
          ${error ? html`<span class="topbar-error">⚠ ${error}</span>` : null}
          ${updated ? html`<span class="topbar-updated">Updated ${updated.toLocaleTimeString()}</span>` : null}
          <form method="POST" action="/logout" style="margin:0">
            <button type="submit" class="logout-btn">Sign out</button>
          </form>
        </div>
      </header>
      <main class="main">
        <${MonitorsSection} monitors=${monitors} loading=${loading} />
        <${MetricsSection}  data=${metrics}      loading=${loading} />
        <${EventsSection}   events=${events}     loading=${loading} />
      </main>
    </div>`;
}

render(html`<${App} />`, document.getElementById('app'));
