import type { SystemRecord, TrafficCountMode, TrafficUsage } from "@/types"

export const DECIMAL_GB = 1_000_000_000n
export const DECIMAL_TB = 1_000_000_000_000n
export const MAX_TRAFFIC_QUOTA = 18_446_744_073_709_551_615n
const DECIMAL_MB = 1_000_000n

export function parseByteString(value?: string): bigint {
	return value && /^\d+$/.test(value) ? BigInt(value) : 0n
}

export function decimalQuotaToBytes(value: string, unit: "GB" | "TB"): bigint | null {
	const match = value.trim().match(/^(\d+)(?:\.(\d+))?$/)
	if (!match) return null
	const scale = unit === "TB" ? DECIMAL_TB : DECIMAL_GB
	const fraction = (match[2] ?? "").replace(/0+$/, "")
	const divisor = 10n ** BigInt(fraction.length)
	const scaled = BigInt(match[1]) * scale + (fraction ? (BigInt(fraction) * scale) / divisor : 0n)
	if (fraction && (BigInt(fraction) * scale) % divisor !== 0n) return null
	if (scaled > MAX_TRAFFIC_QUOTA) return null
	return scaled
}

export function bytesToQuotaInput(bytes: bigint): { value: string; unit: "GB" | "TB" } {
	const unit = bytes >= DECIMAL_TB ? "TB" : "GB"
	const scale = unit === "TB" ? DECIMAL_TB : DECIMAL_GB
	const whole = bytes / scale
	const remainder = bytes % scale
	if (!remainder) return { value: whole.toString(), unit }
	return {
		value: `${whole}.${remainder
			.toString()
			.padStart(scale.toString().length - 1, "0")
			.replace(/0+$/, "")}`,
		unit,
	}
}

export function formatDecimalBytes(bytes: bigint, maximumFractionDigits = 2, locale?: string): string {
	const scale = bytes >= DECIMAL_TB ? DECIMAL_TB : bytes >= DECIMAL_GB ? DECIMAL_GB : DECIMAL_MB
	const unit = scale === DECIMAL_TB ? "TB" : scale === DECIMAL_GB ? "GB" : "MB"
	const factor = 10n ** BigInt(maximumFractionDigits)
	const rounded = (bytes * factor + scale / 2n) / scale
	const whole = rounded / factor
	const fraction = (rounded % factor).toString().padStart(maximumFractionDigits, "0").replace(/0+$/, "")
	const value = fraction ? `${whole}.${fraction}` : whole.toString()
	return `${new Intl.NumberFormat(locale, { maximumFractionDigits, useGrouping: true }).format(value)} ${unit}`
}

export function getTrafficMeterClass(percent: number): string {
	if (percent >= 100) return "bg-red-500"
	if (percent >= 80) return "bg-yellow-500"
	return "bg-green-500"
}

export function getTrafficUsed(usage?: TrafficUsage, mode: TrafficCountMode = "combined"): bigint {
	const sent = parseByteString(usage?.sent_bytes)
	return mode === "egress" ? sent : sent + parseByteString(usage?.recv_bytes)
}

export function getTrafficPercent(used: bigint, quota: bigint): number {
	if (quota <= 0n) return 0
	const tenths = (used * 1_000n) / quota
	return Number(tenths > BigInt(Number.MAX_SAFE_INTEGER) ? BigInt(Number.MAX_SAFE_INTEGER) : tenths) / 10
}

export function hasTrafficQuota(system: SystemRecord): boolean {
	return parseByteString(system.traffic_quota_bytes) > 0n
}
