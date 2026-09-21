import { Cpu, MemoryStick, MonitorCog } from 'lucide-react'
import type { GPUInfo, SystemInfo } from '@/lib/types'
import { formatBytes } from '@/lib/utils'

const percentage = (value: number | null | undefined) => typeof value === 'number' && Number.isFinite(value) ? `${value.toFixed(1)}%` : '—'
const bytes = (value: number | null | undefined) => typeof value === 'number' ? formatBytes(value) : '—'

function UsageBar({ value, label }: { value?: number | null; label: string }) {
  const valid = typeof value === 'number' && Number.isFinite(value)
  return <div role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={valid ? Math.min(100, Math.max(0, value)) : undefined} aria-valuetext={valid ? percentage(value) : 'Unavailable'} className="mt-3 h-1.5 overflow-hidden rounded-full bg-muted">
    <div className="h-full rounded-full bg-primary transition-[width] duration-500" style={{ width: valid ? `${Math.min(100, Math.max(0, value))}%` : '0%' }} />
  </div>
}

export function ResourceMetrics({ info }: { info: SystemInfo | null }) {
  const total = info?.memory.total_bytes
  const available = info?.memory.available_bytes
  const used = typeof total === 'number' && total > 0 && typeof available === 'number' ? Math.max(0, total - available) : undefined
  const memoryPercent = used !== undefined && total ? used / total * 100 : undefined
  const devices = info?.gpu?.devices ?? []
  return <section aria-label="Live hardware usage" className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
    <div className="reference-panel p-5">
      <div className="flex items-center gap-2 text-xs text-muted-foreground"><Cpu className="size-4 text-primary" /> CPU utilization <span className="ml-auto font-mono text-[10px]">{info ? `${info.cpus} logical cores` : 'Loading'}</span></div>
      <div className="reference-title mt-4 text-4xl tabular-nums">{percentage(info?.cpu?.usage_percent)}</div>
      <UsageBar label="CPU utilization" value={info?.cpu?.usage_percent} />
      <p className="mt-3 text-[11px] text-muted-foreground">{info?.cpu?.error || (typeof info?.cpu?.usage_percent === 'number' ? 'Host-wide utilization · all processes' : info ? 'CPU measurement unavailable or awaiting first sample.' : 'Waiting for hardware telemetry…')}</p>
    </div>
    <div className="reference-panel p-5">
      <div className="flex items-center gap-2 text-xs text-muted-foreground"><MemoryStick className="size-4 text-primary" /> System memory <span className="ml-auto text-[10px]">RAM</span></div>
      <div className="reference-title mt-4 text-4xl tabular-nums">{percentage(memoryPercent)}</div>
      <UsageBar label="Physical memory utilization" value={memoryPercent} />
      <p className="mt-3 text-[11px] text-muted-foreground">{used !== undefined ? `${bytes(used)} used / ${bytes(total)} total · ${bytes(available)} available` : info ? 'Physical memory telemetry unavailable on this host.' : 'Waiting for memory telemetry…'}</p>
    </div>
    {devices.length ? devices.map(device => <GPUCard key={device.uuid || device.index} device={device} />) : <div className="reference-panel p-5">
      <div className="flex items-center gap-2 text-xs text-muted-foreground"><MonitorCog className="size-4 text-primary" /> NVIDIA GPU</div>
      <div className="reference-title mt-4 text-2xl">{info ? 'Unavailable' : 'Detecting…'}</div>
      <p className="mt-3 text-xs leading-5 text-muted-foreground">{info?.gpu?.error || (info ? 'GPU telemetry was not reported by the server.' : 'Checking GPU utilization and VRAM through nvidia-smi.')}</p>
    </div>}
  </section>
}

function GPUCard({ device }: { device: GPUInfo }) {
  const used = device.memory_used_bytes
  const total = device.memory_total_bytes
  const memoryPercent = typeof used === 'number' && total ? used / total * 100 : undefined
  return <div className="reference-panel p-5">
    <div className="flex items-center gap-2 text-xs text-muted-foreground"><MonitorCog className="size-4 shrink-0 text-primary" /><span className="truncate" title={device.name}>{device.name}</span><span className="ml-auto shrink-0 font-mono text-[10px]">GPU {device.index}</span></div>
    <div className="mt-4 grid grid-cols-2 gap-4">
      <div><div className="reference-title text-3xl tabular-nums">{percentage(device.utilization_percent)}</div><div className="mt-1 text-[10px] text-muted-foreground">GPU utilization</div><UsageBar label={`${device.name} GPU utilization`} value={device.utilization_percent} /></div>
      <div><div className="reference-title text-3xl tabular-nums">{percentage(memoryPercent)}</div><div className="mt-1 text-[10px] text-muted-foreground">VRAM usage</div><UsageBar label={`${device.name} VRAM usage`} value={memoryPercent} /></div>
    </div>
    <p className="mt-3 text-[11px] text-muted-foreground">{bytes(used)} / {bytes(total)} VRAM</p>
    <p className="mt-1 text-[10px] text-muted-foreground">{[device.driver_version && `Driver ${device.driver_version}`, typeof device.temperature_c === 'number' && `${device.temperature_c}°C`, typeof device.power_watts === 'number' && `${device.power_watts.toFixed(0)} W`].filter(Boolean).join(' · ')}</p>
  </div>
}
