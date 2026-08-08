/** biome-ignore-all lint/correctness/useHookAtTopLevel: Hooks live inside memoized column definitions */
import { plural, t } from "@lingui/core/macro"
import { i18n } from "@lingui/core"
import { Trans, useLingui } from "@lingui/react/macro"
import { useStore } from "@nanostores/react"
import { getPagePath } from "@nanostores/router"
import type { CellContext, ColumnDef, HeaderContext } from "@tanstack/react-table"
import type { ClassValue } from "clsx"
import {
	ArrowDownIcon,
	ArrowUpIcon,
	ArrowUpDownIcon,
	CalendarClockIcon,
	ChevronRightSquareIcon,
	ClockArrowUp,
	CopyIcon,
	CpuIcon,
	HardDriveIcon,
	GaugeIcon,
	MemoryStickIcon,
	MoreHorizontalIcon,
	PauseCircleIcon,
	PenBoxIcon,
	PlayCircleIcon,
	ServerIcon,
	TerminalSquareIcon,
	Trash2Icon,
	WifiIcon,
} from "lucide-react"
import { memo, useMemo, useRef, useState } from "react"
import { Tooltip, TooltipContent, TooltipTrigger } from "../ui/tooltip"
import { isReadOnlyUser, pb } from "@/lib/api"
import { BatteryState, ConnectionType, connectionTypeLabels, MeterState, SystemStatus } from "@/lib/enums"
import { $longestSystemNameLen, $userSettings } from "@/lib/stores"
import {
	cn,
	copyToClipboard,
	decimalString,
	formatBytes,
	formatTemperature,
	parseSemVer,
	secondsToUptimeString,
} from "@/lib/utils"
import {
	formatDecimalBytes,
	getTrafficMeterClass,
	getTrafficPercent,
	getTrafficUsed,
	parseByteString,
} from "@/lib/traffic"
import { batteryStateTranslations } from "@/lib/i18n"
import type { SystemRecord } from "@/types"
import { TrafficQuotaSettings } from "../routes/system/traffic-quota"
import { SystemDialog } from "../add-system"
import AlertButton from "../alerts/alert-button"
import { $router, Link } from "../router"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "../ui/alert-dialog"
import { Button, buttonVariants } from "../ui/button"
import { Dialog } from "../ui/dialog"
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "../ui/dropdown-menu"
import {
	BatteryMediumIcon,
	EthernetIcon,
	GpuIcon,
	HourglassIcon,
	ThermometerIcon,
	WebSocketIcon,
	BatteryHighIcon,
	BatteryLowIcon,
	PlugChargingIcon,
	BatteryFullIcon,
} from "../ui/icons"

const STATUS_COLORS = {
	[SystemStatus.Up]: "bg-green-500",
	[SystemStatus.Down]: "bg-red-500",
	[SystemStatus.Paused]: "bg-primary/40",
	[SystemStatus.Pending]: "bg-yellow-500",
} as const

function getMeterStateByThresholds(value: number, warn = 65, crit = 90): MeterState {
	return value >= crit ? MeterState.Crit : value >= warn ? MeterState.Warn : MeterState.Good
}

/**
 * @param viewMode - "table" or "grid"
 * @returns - Column definitions for the systems table
 */
export function SystemsTableColumns(viewMode: "table" | "grid"): ColumnDef<SystemRecord>[] {
	return [
		{
			size: 220,
			accessorKey: "name",
			id: "system",
			name: () => t`System`,
			sortingFn: (a, b) => a.original.name.localeCompare(b.original.name),
			filterFn: (() => {
				let filterInput = ""
				let filterInputLower = ""
				const nameCache = new Map<string, string>()
				const statusTranslations = {
					[SystemStatus.Up]: t`Up`.toLowerCase(),
					[SystemStatus.Down]: t`Down`.toLowerCase(),
					[SystemStatus.Paused]: t`Paused`.toLowerCase(),
				} as const

				// match filter value against name or translated status
				return (row, _, newFilterInput) => {
					const sys = row.original
					if (sys.host.includes(newFilterInput) || sys.info.v?.includes(newFilterInput)) {
						return true
					}
					if (newFilterInput !== filterInput) {
						filterInput = newFilterInput
						filterInputLower = newFilterInput.toLowerCase()
					}
					let nameLower = nameCache.get(sys.name)
					if (nameLower === undefined) {
						nameLower = sys.name.toLowerCase()
						nameCache.set(sys.name, nameLower)
					}
					if (nameLower.includes(filterInputLower)) {
						return true
					}
					const statusLower = statusTranslations[sys.status as keyof typeof statusTranslations]
					return statusLower?.includes(filterInputLower) || false
				}
			})(),
			enableHiding: false,
			invertSorting: false,
			Icon: ServerIcon,
			cell: (info) => {
				const { name, id } = info.row.original
				const longestName = useStore($longestSystemNameLen)
				const linkUrl = getPagePath($router, "system", { id })

				return (
					<>
						<span className="flex gap-2 items-center font-medium text-sm text-nowrap md:ps-1">
							<IndicatorDot system={info.row.original} />
							<Link
								href={linkUrl}
								tabIndex={-1}
								className="truncate z-10 relative"
								style={{ width: `${longestName / 1.05}ch` }}
								onMouseEnter={(e) => {
									// set title on hover if text is truncated to show full name
									const a = e.currentTarget
									if (a.scrollWidth > a.clientWidth) {
										a.title = name
									} else {
										a.removeAttribute("title")
									}
								}}
							>
								{name}
							</Link>
						</span>
						<Link href={linkUrl} className="inset-0 absolute size-full" aria-label={name}></Link>
					</>
				)
			},
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.cpu || undefined,
			id: "cpu",
			size: 160,
			name: () => t`CPU`,
			cell: TableCellWithMeter,
			Icon: CpuIcon,
			header: sortableHeader,
		},
		{
			// accessorKey: "info.mp",
			accessorFn: ({ info }) => info.mp || undefined,
			id: "memory",
			size: 220,
			name: () => t`Memory`,
			cell: (info: CellContext<SystemRecord, unknown>) => TableCellWithMeter(info, info.row.original.info.ms),
			Icon: MemoryStickIcon,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.dp || undefined,
			id: "disk",
			size: 220,
			name: () => t`Disk`,
			cell: (info: CellContext<SystemRecord, unknown>) =>
				info.row.original.info.efs ? DiskCellWithMultiple(info) : TableCellWithMeter(info, info.row.original.info.ds),
			Icon: HardDriveIcon,
			header: sortableHeader,
		},
		{
			accessorFn: ({ info }) => info.g || undefined,
			id: "gpu",
			size: 160,
			name: () => "GPU",
			cell: TableCellWithMeter,
			Icon: GpuIcon,
			header: sortableHeader,
		},
		{
			id: "loadAverage",
			accessorFn: ({ info }) => info.la?.reduce((acc, curr) => acc + curr, 0),
			name: () => t({ message: "Load Avg", comment: "Short label for load average" }),
			size: 150,
			Icon: HourglassIcon,
			header: sortableHeader,
			cell(info: CellContext<SystemRecord, unknown>) {
				const { info: sysInfo, status } = info.row.original
				const { major, minor } = parseSemVer(sysInfo.v)
				const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
				const loadAverages = sysInfo.la || []

				const max = Math.max(...loadAverages)
				if (max === 0 && (status === SystemStatus.Paused || (major < 1 && minor < 13))) {
					return null
				}

				const normalizedLoad = max / (sysInfo.t ?? 1)
				const threshold = getMeterStateByThresholds(normalizedLoad * 100, colorWarn, colorCrit)

				return (
					<div className="flex items-center gap-[.35em] w-full tabular-nums tracking-tight">
						<span
							className={cn("inline-block size-2 rounded-full me-0.5", {
								[STATUS_COLORS[SystemStatus.Up]]: threshold === MeterState.Good,
								[STATUS_COLORS[SystemStatus.Pending]]: threshold === MeterState.Warn,
								[STATUS_COLORS[SystemStatus.Down]]: threshold === MeterState.Crit,
								[STATUS_COLORS[SystemStatus.Paused]]: status !== SystemStatus.Up,
							})}
						/>
						{loadAverages?.map((la, i) => (
							<span key={i}>{decimalString(la, la >= 10 ? 1 : 2)}</span>
						))}
					</div>
				)
			},
		},
		{
			accessorFn: ({ info, status }) => (status !== SystemStatus.Up ? undefined : info.bb),
			id: "net",
			name: () => t`Net`,
			size: 120,
			Icon: EthernetIcon,
			header: sortableHeader,
			sortUndefined: "last",
			cell(info) {
				const val = info.getValue() as number | undefined
				if (val === undefined) {
					return null
				}
				const userSettings = useStore($userSettings, { keys: ["unitNet"] })
				const { value, unit } = formatBytes(val, true, userSettings.unitNet, false)
				return (
					<span className="tabular-nums whitespace-nowrap">
						{decimalString(value, value >= 100 ? 1 : 2)} {unit}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.dt,
			id: "temp",
			name: () => t({ message: "Temp", comment: "Temperature label in systems table" }),
			size: 100,
			hideSort: true,
			Icon: ThermometerIcon,
			header: sortableHeader,
			cell(info) {
				const val = info.getValue() as number
				const userSettings = useStore($userSettings, { keys: ["unitTemp"] })
				if (!val) {
					return null
				}
				const { value, unit } = formatTemperature(val, userSettings.unitTemp)
				return (
					<span className={cn("tabular-nums whitespace-nowrap", viewMode === "table" && "ps-0.5")}>
						{decimalString(value, value >= 100 ? 1 : 2)} {unit}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.bat?.[0],
			id: "battery",
			name: () => t({ message: "Bat", comment: "Battery label in systems table header" }),
			size: 90,
			Icon: BatteryMediumIcon,
			header: sortableHeader,
			hideSort: true,
			cell(info) {
				const [pct, state] = info.row.original.info.bat ?? []
				if (pct === undefined) {
					return null
				}

				let Icon = PlugChargingIcon
				let iconColor = "text-muted-foreground"

				if (state !== BatteryState.Charging) {
					if (pct < 25) {
						iconColor = pct < 11 ? "text-red-500" : "text-yellow-500"
						Icon = BatteryLowIcon
					} else if (pct < 75) {
						Icon = BatteryMediumIcon
					} else if (pct < 95) {
						Icon = BatteryHighIcon
					} else {
						Icon = BatteryFullIcon
					}
				}

				const stateLabel =
					state !== undefined ? (batteryStateTranslations[state as BatteryState]?.() ?? undefined) : undefined

				return (
					<Link
						tabIndex={-1}
						href={getPagePath($router, "system", { id: info.row.original.id })}
						className="flex items-center gap-1 tabular-nums tracking-tight relative z-10"
						title={stateLabel}
					>
						<Icon className={cn("size-3.5", iconColor)} />
						<span className="min-w-10">{pct}%</span>
					</Link>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.sv?.[0],
			id: "services",
			name: () => t`Services`,
			size: 170,
			Icon: TerminalSquareIcon,
			header: sortableHeader,
			hideSort: true,
			sortingFn: (a, b) => {
				// sort priorities: 1) failed services, 2) total services
				const [totalCountA, numFailedA] = a.original.info.sv ?? [0, 0]
				const [totalCountB, numFailedB] = b.original.info.sv ?? [0, 0]
				if (numFailedA !== numFailedB) {
					return numFailedA - numFailedB
				}
				return totalCountA - totalCountB
			},
			cell(info) {
				const sys = info.row.original
				const [totalCount, numFailed] = sys.info.sv ?? [0, 0]
				if (sys.status !== SystemStatus.Up || totalCount === 0) {
					return null
				}
				return (
					<span className="tabular-nums whitespace-nowrap flex gap-1.5 items-center">
						<span
							className={cn("block size-2 rounded-full", {
								[STATUS_COLORS[SystemStatus.Down]]: numFailed > 0,
								[STATUS_COLORS[SystemStatus.Up]]: numFailed === 0,
							})}
						/>
						{totalCount}{" "}
						<span className="text-muted-foreground text-sm -ms-0.5">
							({t`Failed`.toLowerCase()}: {numFailed})
						</span>
					</span>
				)
			},
		},
		{
			accessorFn: (system) => {
				const quota = parseByteString(system.traffic_quota_bytes)
				return quota > 0n
					? getTrafficPercent(getTrafficUsed(system.traffic_usage, system.traffic_count_mode), quota)
					: undefined
			},
			id: "traffic",
			name: () => t`Traffic`,
			size: 220,
			Icon: GaugeIcon,
			header: sortableHeader,
			sortUndefined: "last",
			sortingFn: (a, b) => {
				const quotaA = parseByteString(a.original.traffic_quota_bytes)
				const quotaB = parseByteString(b.original.traffic_quota_bytes)
				if (quotaA === 0n || quotaB === 0n) return quotaA === quotaB ? 0 : quotaA === 0n ? -1 : 1
				const usedA = getTrafficUsed(a.original.traffic_usage, a.original.traffic_count_mode)
				const usedB = getTrafficUsed(b.original.traffic_usage, b.original.traffic_count_mode)
				const left = usedA * quotaB
				const right = usedB * quotaA
				return left === right ? 0 : left < right ? -1 : 1
			},
			cell(info) {
				const system = info.row.original
				const quota = parseByteString(system.traffic_quota_bytes)
				if (quota === 0n) {
					return <span className="whitespace-nowrap text-muted-foreground">{t`Not set`}</span>
				}
				const used = getTrafficUsed(system.traffic_usage, system.traffic_count_mode)
				const percentage = getTrafficPercent(used, quota)
				const displayPercentage = Math.min(percentage, 999.9)
				const compactUsed = formatDecimalBytes(used, 1, i18n.locale).replace(/\s?([MGT])B$/, "$1")
				const compactQuota = formatDecimalBytes(quota, 1, i18n.locale).replace(/\s?([MGT])B$/, "$1")
				return (
					<div
						className="flex w-full items-center gap-2 tabular-nums tracking-tight"
						title={`${formatDecimalBytes(used, 2, i18n.locale)} / ${formatDecimalBytes(quota, 2, i18n.locale)}`}
					>
						<span className="w-11 shrink-0">{displayPercentage}%</span>
						<MeterBar
							value={percentage}
							fillClass={getTrafficMeterClass(percentage)}
							className="w-32 shrink-0"
							role="progressbar"
							aria-label={t`Monthly traffic quota used`}
							aria-valuemin={0}
							aria-valuemax={100}
							aria-valuenow={Math.min(percentage, 100)}
							aria-valuetext={`${formatDecimalBytes(used, 2, i18n.locale)} / ${formatDecimalBytes(quota, 2, i18n.locale)} (${percentage}%)`}
						>
							<span className="relative z-10 mx-auto whitespace-nowrap text-[10px] font-medium leading-none drop-shadow-[0_1px_1px_hsl(var(--background))]">
								{compactUsed} / {compactQuota}
							</span>
						</MeterBar>
					</div>
				)
			},
		},
		{
			accessorFn: ({ subscription_expires }) => {
				const expiresAt = Date.parse(subscription_expires ?? "")
				return Number.isNaN(expiresAt) ? undefined : expiresAt
			},
			id: "subscription",
			name: () => t`Subscription`,
			size: 130,
			Icon: CalendarClockIcon,
			header: sortableHeader,
			sortUndefined: "last",
			cell(info) {
				const expiresAt = info.getValue() as number | undefined
				if (expiresAt === undefined) {
					return <span className="text-muted-foreground whitespace-nowrap">{t`Not set`}</span>
				}
				const remainingDays = Math.ceil((expiresAt - Date.now()) / 86_400_000)
				if (remainingDays <= 0) {
					return <span className="text-red-500 whitespace-nowrap">{t`Expired`}</span>
				}
				return (
					<span className={cn("tabular-nums whitespace-nowrap", remainingDays <= 30 && "text-yellow-500")}>
						{plural(remainingDays, { one: "# day", other: "# days" })}
					</span>
				)
			},
		},
		{
			accessorFn: ({ info }) => info.u || undefined,
			id: "uptime",
			name: () => t`Uptime`,
			size: 120,
			Icon: ClockArrowUp,
			header: sortableHeader,
			hideSort: true,
			cell(info) {
				const uptime = info.getValue() as number
				if (!uptime) {
					return null
				}
				return <span className="tabular-nums whitespace-nowrap">{secondsToUptimeString(uptime)}</span>
			},
		},
		{
			accessorFn: ({ info }) => info.v,
			id: "agent",
			name: () => t`Agent`,
			size: 100,
			Icon: WifiIcon,
			hideSort: true,
			header: sortableHeader,
			cell(info) {
				const version = info.getValue() as string
				if (!version) {
					return null
				}
				const system = info.row.original
				const color = {
					"text-green-500": version === globalThis.BESZEL.HUB_VERSION,
					"text-yellow-500": version !== globalThis.BESZEL.HUB_VERSION,
					"text-red-500": system.status !== SystemStatus.Up,
				}
				return (
					<Link
						href={getPagePath($router, "system", { id: system.id })}
						className={cn(
							"flex gap-1.5 items-center md:pe-5 tabular-nums relative z-10",
							viewMode === "table" && "ps-0.5"
						)}
						tabIndex={-1}
						title={connectionTypeLabels[system.info.ct as ConnectionType]}
						role="none"
					>
						{system.info.ct === ConnectionType.WebSocket && (
							<WebSocketIcon className={cn("size-3 pointer-events-none", color)} />
						)}
						{system.info.ct === ConnectionType.SSH && (
							<ChevronRightSquareIcon className={cn("size-3 pointer-events-none", color)} />
						)}
						{!system.info.ct && <IndicatorDot system={system} className={cn(color, "bg-current mx-0.5")} />}
						<span className="truncate max-w-14">{info.getValue() as string}</span>
					</Link>
				)
			},
		},
		{
			id: "actions",
			// @ts-expect-error
			name: () => t({ message: "Actions", comment: "Table column" }),
			size: 96,
			cell: ({ row }) => (
				<div className="relative z-10 flex justify-end items-center gap-1 -ms-3">
					<AlertButton system={row.original} />
					<ActionsButton system={row.original} />
				</div>
			),
		},
	] as ColumnDef<SystemRecord>[]
}

function sortableHeader(context: HeaderContext<SystemRecord, unknown>) {
	const { column } = context
	// @ts-expect-error
	const { Icon, hideSort, name }: { Icon: React.ElementType; name: () => string; hideSort: boolean } = column.columnDef
	const isSorted = column.getIsSorted()
	return (
		<Button
			variant="ghost"
			className={cn("h-9 px-3 flex duration-50 hover:bg-transparent", isSorted && "text-foreground")}
			onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}
		>
			{Icon && <Icon className="me-2 size-4" />}
			{name()}
			{hideSort ||
				(isSorted === "asc" ? (
					<ArrowUpIcon className="ms-2 size-4" />
				) : isSorted === "desc" ? (
					<ArrowDownIcon className="ms-2 size-4" />
				) : (
					<ArrowUpDownIcon className="ms-2 size-4 opacity-60" />
				))}
		</Button>
	)
}

function TableCellWithMeter(info: CellContext<SystemRecord, unknown>, usage?: [number, number]) {
	const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
	const val = Number(info.getValue()) || 0
	const threshold = getMeterStateByThresholds(val, colorWarn, colorCrit)
	const meterClass = cn(
		"h-full",
		(info.row.original.status !== SystemStatus.Up && STATUS_COLORS.paused) ||
			(threshold === MeterState.Good && STATUS_COLORS.up) ||
			(threshold === MeterState.Warn && STATUS_COLORS.pending) ||
			STATUS_COLORS.down
	)
	return (
		<div className="flex gap-2 items-center tabular-nums tracking-tight w-full">
			<span className="w-11 shrink-0">{decimalString(val, val >= 10 ? 1 : 2)}%</span>
			<MeterBar value={val} fillClass={meterClass} className={usage ? "w-32 shrink-0" : "flex-1 min-w-8"}>
				{usage?.[1] ? <UsageTotal usage={usage} /> : null}
			</MeterBar>
		</div>
	)
}

function MeterBar({
	value,
	fillClass,
	className,
	children,
	...props
}: React.ComponentProps<"span"> & { value: number; fillClass: string }) {
	return (
		<span
			className={cn(
				"relative flex h-[1em] items-center justify-center overflow-hidden rounded-sm bg-muted px-1",
				className
			)}
			{...props}
		>
			<span
				className={cn("absolute inset-y-0 start-0", fillClass)}
				style={{ width: `${Math.min(Math.max(value, 0), 100)}%` }}
			/>
			{children}
		</span>
	)
}

function DiskCellWithMultiple(info: CellContext<SystemRecord, unknown>) {
	const { colorWarn = 65, colorCrit = 90 } = useStore($userSettings, { keys: ["colorWarn", "colorCrit"] })
	const { info: sysInfo, status, id } = info.row.original
	const extraFs = Object.entries(sysInfo.efs ?? {})
	const rootDiskPct = sysInfo.dp

	// sort extra disks by percentage descending
	extraFs.sort((a, b) => b[1] - a[1])

	function getIndicatorColor(pct: number) {
		const threshold = getMeterStateByThresholds(pct, colorWarn, colorCrit)
		return (
			(status !== SystemStatus.Up && STATUS_COLORS.paused) ||
			(threshold === MeterState.Good && STATUS_COLORS.up) ||
			(threshold === MeterState.Warn && STATUS_COLORS.pending) ||
			STATUS_COLORS.down
		)
	}

	function getMeterClass(pct: number) {
		return cn("h-full", getIndicatorColor(pct))
	}

	// Extra disk indicators (max 3 dots - one per state if any disk exists in range)
	const stateColors = [STATUS_COLORS.up, STATUS_COLORS.pending, STATUS_COLORS.down]
	const extraDiskIndicators =
		status !== SystemStatus.Up
			? []
			: [...new Set(extraFs.map(([, pct]) => getMeterStateByThresholds(pct, colorWarn, colorCrit)))]
					.sort()
					.map((state) => stateColors[state])

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Link
					href={getPagePath($router, "system", { id })}
					tabIndex={-1}
					className="flex flex-col gap-0.5 w-full relative z-10"
				>
					<div className="flex gap-2 items-center tabular-nums tracking-tight">
						<span className="w-11 shrink-0">{decimalString(rootDiskPct, rootDiskPct >= 10 ? 1 : 2)}%</span>
						<MeterBar value={rootDiskPct} fillClass={getMeterClass(rootDiskPct)} className="w-32 shrink-0">
							{sysInfo.ds?.[1] ? <UsageTotal usage={sysInfo.ds} /> : null}
							{/* Extra disk indicators */}
							<span className="absolute end-1 z-10 flex items-center gap-0.5">
								{extraDiskIndicators.map((color) => (
									<span
										key={color}
										className={cn("size-1.5 rounded-full shrink-0 outline-[0.5px] outline-muted", color)}
									/>
								))}
							</span>
						</MeterBar>
					</div>
				</Link>
			</TooltipTrigger>
			<TooltipContent side="right" className="max-w-xs pb-2">
				<div className="grid gap-1">
					<div className="grid gap-0.5">
						<div className="text-[0.65rem] text-muted-foreground uppercase tracking-wide tabular-nums">
							<Trans context="Root disk label">Root</Trans>
						</div>
						<div className="flex gap-2 items-center tabular-nums text-xs">
							<span className="min-w-7">{decimalString(rootDiskPct, rootDiskPct >= 10 ? 1 : 2)}%</span>
							<span className="flex-1 min-w-12 grid bg-muted h-2.5 rounded-sm overflow-hidden">
								<span className={getMeterClass(rootDiskPct)} style={{ width: `${rootDiskPct}%` }}></span>
							</span>
						</div>
					</div>
					{extraFs.map(([name, pct]) => {
						return (
							<div key={name} className="grid gap-0.5">
								<div className="text-[0.65rem] max-w-40 text-muted-foreground uppercase tracking-wide truncate">
									{name}
								</div>
								<div className="flex gap-2 items-center tabular-nums text-xs">
									<span className="min-w-7">{decimalString(pct, pct >= 10 ? 1 : 2)}%</span>
									<span className="flex-1 min-w-12 grid bg-muted h-2.5 rounded-sm overflow-hidden">
										<span className={getMeterClass(pct)} style={{ width: `${pct}%` }}></span>
									</span>
								</div>
							</div>
						)
					})}
				</div>
			</TooltipContent>
		</Tooltip>
	)
}

function UsageTotal({ usage: [used, total] }: { usage: [number, number] }) {
	const useTerabytes = total >= 1000
	const divisor = useTerabytes ? 1024 : 1
	const unit = useTerabytes ? "TB" : "GB"
	const usedValue = used / divisor
	const totalValue = total / divisor

	return (
		<span className="relative z-10 mx-auto whitespace-nowrap px-1 text-[10px] font-medium leading-none drop-shadow-[0_1px_1px_hsl(var(--background))]">
			{decimalString(usedValue, usedValue >= 10 ? 1 : 2)} / {decimalString(totalValue, totalValue >= 10 ? 1 : 2)} {unit}
		</span>
	)
}

export function IndicatorDot({ system, className }: { system: SystemRecord; className?: ClassValue }) {
	className ||= STATUS_COLORS[system.status as keyof typeof STATUS_COLORS] || ""
	return (
		<span
			className={cn("shrink-0 size-2 rounded-full", className)}
			// style={{ marginBottom: "-1px" }}
		/>
	)
}

export const ActionsButton = memo(({ system }: { system: SystemRecord }) => {
	const [deleteOpen, setDeleteOpen] = useState(false)
	const [editOpen, setEditOpen] = useState(false)
	const [trafficOpen, setTrafficOpen] = useState(false)
	const editOpened = useRef(false)
	const { t } = useLingui()
	const { id, status, host, name } = system

	return useMemo(() => {
		return (
			<>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button variant="ghost" size={"icon"}>
							<span className="sr-only">
								<Trans>Open menu</Trans>
							</span>
							<MoreHorizontalIcon className="w-5" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						{!isReadOnlyUser() && (
							<>
								<DropdownMenuItem
									onSelect={() => {
										editOpened.current = true
										setEditOpen(true)
									}}
								>
									<PenBoxIcon className="me-2.5 size-4" />
									<Trans>Edit</Trans>
								</DropdownMenuItem>
								<DropdownMenuItem onSelect={() => setTrafficOpen(true)}>
									<GaugeIcon className="me-2.5 size-4" />
									<Trans>Traffic settings</Trans>
								</DropdownMenuItem>
							</>
						)}
						<DropdownMenuItem
							className={cn(isReadOnlyUser() && "hidden")}
							onClick={() => {
								pb.collection("systems").update(id, {
									status: status === SystemStatus.Paused ? SystemStatus.Pending : SystemStatus.Paused,
								})
							}}
						>
							{status === SystemStatus.Paused ? (
								<>
									<PlayCircleIcon className="me-2.5 size-4" />
									<Trans>Resume</Trans>
								</>
							) : (
								<>
									<PauseCircleIcon className="me-2.5 size-4" />
									<Trans>Pause</Trans>
								</>
							)}
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => copyToClipboard(name)}>
							<CopyIcon className="me-2.5 size-4" />
							<Trans>Copy name</Trans>
						</DropdownMenuItem>
						<DropdownMenuItem onClick={() => copyToClipboard(host)}>
							<CopyIcon className="me-2.5 size-4" />
							<Trans>Copy host</Trans>
						</DropdownMenuItem>
						<DropdownMenuSeparator className={cn(isReadOnlyUser() && "hidden")} />
						<DropdownMenuItem className={cn(isReadOnlyUser() && "hidden")} onSelect={() => setDeleteOpen(true)}>
							<Trash2Icon className="me-2.5 size-4" />
							<Trans>Delete</Trans>
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
				{/* edit dialog */}
				<Dialog open={editOpen} onOpenChange={setEditOpen}>
					{editOpened.current && <SystemDialog system={system} setOpen={setEditOpen} />}
				</Dialog>
				<TrafficQuotaSettings system={system} open={trafficOpen} onOpenChange={setTrafficOpen} hideTrigger />
				{/* deletion dialog */}
				<AlertDialog open={deleteOpen} onOpenChange={(open) => setDeleteOpen(open)}>
					<AlertDialogContent>
						<AlertDialogHeader>
							<AlertDialogTitle>
								<Trans>Are you sure you want to delete {name}?</Trans>
							</AlertDialogTitle>
							<AlertDialogDescription>
								<Trans>
									This action cannot be undone. This will permanently delete all current records for {name} from the
									database.
								</Trans>
							</AlertDialogDescription>
						</AlertDialogHeader>
						<AlertDialogFooter>
							<AlertDialogCancel>
								<Trans>Cancel</Trans>
							</AlertDialogCancel>
							<AlertDialogAction
								className={cn(buttonVariants({ variant: "destructive" }))}
								onClick={() => pb.collection("systems").delete(id)}
							>
								<Trans>Continue</Trans>
							</AlertDialogAction>
						</AlertDialogFooter>
					</AlertDialogContent>
				</AlertDialog>
			</>
		)
	}, [id, status, host, name, system, t, deleteOpen, editOpen, trafficOpen])
})
