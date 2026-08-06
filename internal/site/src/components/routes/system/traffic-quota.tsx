import { Trans, useLingui } from "@lingui/react/macro"
import { CircleAlertIcon, Settings2Icon } from "lucide-react"
import { useEffect, useState } from "react"
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import {
	Sheet,
	SheetContent,
	SheetDescription,
	SheetFooter,
	SheetHeader,
	SheetTitle,
	SheetTrigger,
} from "@/components/ui/sheet"
import { toast } from "@/components/ui/use-toast"
import { isReadOnlyUser, pb } from "@/lib/api"
import {
	bytesToQuotaInput,
	decimalQuotaToBytes,
	formatDecimalBytes,
	getTrafficPercent,
	getTrafficMeterClass,
	getTrafficUsed,
	parseByteString,
} from "@/lib/traffic"
import type { SystemRecord, TrafficCountMode } from "@/types"

function formatCycleDate(value: string | undefined, locale: string) {
	if (!value) return "-"
	const date = new Date(value)
	return Number.isNaN(date.getTime())
		? value
		: new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Shanghai" }).format(
				date
			)
}

export default function TrafficQuota({ system }: { system: SystemRecord }) {
	const { i18n, t } = useLingui()
	const locale = i18n.locale
	const quota = parseByteString(system.traffic_quota_bytes)
	const mode = system.traffic_count_mode ?? "combined"
	const usage = system.traffic_usage
	const sent = parseByteString(usage?.sent_bytes)
	const received = parseByteString(usage?.recv_bytes)
	const used = getTrafficUsed(usage, mode)
	const remaining = quota > used ? quota - used : 0n
	const percentage = getTrafficPercent(used, quota)

	return (
		<Card>
			<CardHeader className="flex-row items-center justify-between gap-3 pb-3">
				<CardTitle className="text-lg">
					<Trans>Monthly traffic</Trans>
				</CardTitle>
				<TrafficQuotaSettings system={system} />
			</CardHeader>
			<CardContent className="grid gap-4">
				{quota > 0n ? (
					<>
						<div>
							<div className="mb-2 flex items-baseline justify-between gap-3 text-sm tabular-nums">
								<span className="font-medium">{formatDecimalBytes(used, 2, locale)}</span>
								<span className="text-muted-foreground">{Math.min(percentage, 999.9)}%</span>
							</div>
							<div
								className="h-2.5 overflow-hidden rounded-full bg-muted"
								role="progressbar"
								aria-label={t`Monthly traffic quota used`}
								aria-valuemin={0}
								aria-valuemax={100}
								aria-valuenow={Math.min(percentage, 100)}
								aria-valuetext={`${formatDecimalBytes(used, 2, locale)} / ${formatDecimalBytes(quota, 2, locale)} (${percentage}%)`}
							>
								<div
									className={`h-full transition-[width] ${getTrafficMeterClass(percentage)}`}
									style={{ width: `${Math.min(percentage, 100)}%` }}
								/>
							</div>
						</div>
						<dl className="grid grid-cols-2 gap-x-6 gap-y-4 text-sm sm:grid-cols-4">
							<TrafficValue label={<Trans>Sent</Trans>} value={formatDecimalBytes(sent, 2, locale)} />
							<TrafficValue label={<Trans>Received</Trans>} value={formatDecimalBytes(received, 2, locale)} />
							<TrafficValue label={<Trans>Used</Trans>} value={formatDecimalBytes(used, 2, locale)} />
							<TrafficValue label={<Trans>Quota</Trans>} value={formatDecimalBytes(quota, 2, locale)} />
							<TrafficValue label={<Trans>Remaining</Trans>} value={formatDecimalBytes(remaining, 2, locale)} />
							<TrafficValue
								label={<Trans>Cycle</Trans>}
								value={`${formatCycleDate(usage?.cycle_start, locale)} - ${formatCycleDate(usage?.cycle_end, locale)}`}
								className="col-span-2"
							/>
							<TrafficValue
								label={<Trans>Tracking started</Trans>}
								value={formatCycleDate(usage?.observed_from, locale)}
							/>
							<TrafficValue label={<Trans>Source</Trans>} value={t`Agent statistics`} />
						</dl>
						{usage && !usage.complete && (
							<div className="flex gap-2 rounded-md border border-yellow-500/30 bg-yellow-500/5 p-3 text-sm text-yellow-700 dark:text-yellow-400">
								<CircleAlertIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
								<div>
									<p className="font-medium">
										<Trans>Traffic data is incomplete</Trans>
									</p>
									<p className="mt-0.5 text-xs opacity-90">
										<Trans>
											The agent observed {usage.reset_count} counter resets and {usage.interface_change_count} interface
											changes during this cycle.
										</Trans>
									</p>
								</div>
							</div>
						)}
					</>
				) : (
					<p className="text-sm text-muted-foreground">
						<Trans>No monthly traffic quota is set.</Trans>
					</p>
				)}
			</CardContent>
		</Card>
	)
}

function TrafficValue({ label, value, className = "" }: { label: React.ReactNode; value: string; className?: string }) {
	return (
		<div className={className}>
			<dt className="text-muted-foreground">{label}</dt>
			<dd className="mt-1 tabular-nums">{value}</dd>
		</div>
	)
}

export function TrafficQuotaSettings({
	system,
	open: controlledOpen,
	onOpenChange,
	hideTrigger = false,
}: {
	system: SystemRecord
	open?: boolean
	onOpenChange?: (open: boolean) => void
	hideTrigger?: boolean
}) {
	const { t } = useLingui()
	const initialQuota = bytesToQuotaInput(parseByteString(system.traffic_quota_bytes))
	const [internalOpen, setInternalOpen] = useState(false)
	const open = controlledOpen ?? internalOpen
	const setOpen = onOpenChange ?? setInternalOpen
	const [confirmOpen, setConfirmOpen] = useState(false)
	const [quotaValue, setQuotaValue] = useState(initialQuota.value)
	const [quotaUnit, setQuotaUnit] = useState<"GB" | "TB">(initialQuota.unit)
	const [cycleDay, setCycleDay] = useState(system.traffic_cycle_day ?? 1)
	const [mode, setMode] = useState<TrafficCountMode>(system.traffic_count_mode ?? "combined")
	const [saving, setSaving] = useState(false)
	const readonly = isReadOnlyUser()
	const parsedQuota = decimalQuotaToBytes(quotaValue, quotaUnit)
	const invalidQuota = parsedQuota === null
	const quotaErrorId = `traffic-quota-error-${system.id}`
	const quotaHelpId = `traffic-quota-help-${system.id}`

	useEffect(() => {
		if (!open) return
		const quota = bytesToQuotaInput(parseByteString(system.traffic_quota_bytes))
		setQuotaValue(quota.value)
		setQuotaUnit(quota.unit)
		setCycleDay(system.traffic_cycle_day ?? 1)
		setMode(system.traffic_count_mode ?? "combined")
	}, [open, system.traffic_quota_bytes, system.traffic_cycle_day, system.traffic_count_mode])

	async function save() {
		if (parsedQuota === null) return
		setSaving(true)
		try {
			await pb.collection("systems").update(system.id, {
				traffic_quota_bytes: parsedQuota.toString(),
				traffic_cycle_day: cycleDay,
				traffic_count_mode: mode,
			})
			setOpen(false)
		} catch (error) {
			console.error(error)
			toast({ title: t`Failed to update traffic quota`, variant: "destructive" })
		} finally {
			setSaving(false)
		}
	}

	function submit(event: React.FormEvent) {
		event.preventDefault()
		if (cycleDay !== (system.traffic_cycle_day ?? 1)) {
			setConfirmOpen(true)
			return
		}
		save()
	}

	return (
		<>
			<Sheet open={open} onOpenChange={setOpen}>
				{!hideTrigger && (
					<SheetTrigger asChild>
						<Button variant="outline" size="sm">
							<Settings2Icon className="me-2 size-4" aria-hidden="true" />
							<Trans>Traffic settings</Trans>
						</Button>
					</SheetTrigger>
				)}
				<SheetContent className="w-full overflow-y-auto p-0 sm:max-w-md">
					<form className="flex min-h-full flex-col" onSubmit={submit}>
						<SheetHeader className="border-b p-6">
							<SheetTitle>
								<Trans>Monthly traffic quota</Trans>
							</SheetTitle>
							<SheetDescription>
								<Trans>Configure the billing cycle and traffic counted for this system.</Trans>
							</SheetDescription>
						</SheetHeader>
						<div className="grid gap-6 p-6">
							<div className="grid gap-2">
								<Label htmlFor={`traffic-quota-${system.id}`}>
									<Trans>Monthly quota</Trans>
								</Label>
								<div className="flex gap-2">
									<Input
										id={`traffic-quota-${system.id}`}
										type="number"
										inputMode="decimal"
										min="0"
										step="any"
										value={quotaValue}
										onChange={(event) => setQuotaValue(event.target.value)}
										disabled={readonly}
										aria-invalid={invalidQuota}
										aria-describedby={`${quotaHelpId}${invalidQuota ? ` ${quotaErrorId}` : ""}`}
									/>
									<Select
										value={quotaUnit}
										onValueChange={(value) => setQuotaUnit(value as "GB" | "TB")}
										disabled={readonly}
									>
										<SelectTrigger className="w-24" aria-label={t`Quota unit`}>
											<SelectValue />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="GB">GB</SelectItem>
											<SelectItem value="TB">TB</SelectItem>
										</SelectContent>
									</Select>
								</div>
								<p id={quotaHelpId} className="text-xs text-muted-foreground">
									<Trans>Decimal units are used: 1 GB = 1,000,000,000 bytes. Set the quota to 0 to disable it.</Trans>
								</p>
								{invalidQuota && (
									<p id={quotaErrorId} className="text-xs text-destructive" role="alert">
										<Trans>Enter a quota that resolves to a whole number of bytes.</Trans>
									</p>
								)}
							</div>
							<div className="grid gap-2">
								<Label htmlFor={`traffic-day-${system.id}`}>
									<Trans>Billing day</Trans>
								</Label>
								<Input
									id={`traffic-day-${system.id}`}
									type="number"
									min={1}
									max={31}
									value={cycleDay}
									onChange={(event) => setCycleDay(Math.max(1, Math.min(31, Number(event.target.value))))}
									disabled={readonly}
								/>
								<p className="text-xs text-muted-foreground">
									<Trans>Cycles use Asia/Shanghai time. For shorter months, day 29-31 uses the month's last day.</Trans>
								</p>
							</div>
							<div className="grid gap-2">
								<Label htmlFor={`traffic-mode-${system.id}`}>
									<Trans>Traffic counted</Trans>
								</Label>
								<Select value={mode} onValueChange={(value) => setMode(value as TrafficCountMode)} disabled={readonly}>
									<SelectTrigger id={`traffic-mode-${system.id}`}>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="combined">
											<Trans>Sent and received</Trans>
										</SelectItem>
										<SelectItem value="egress">
											<Trans>Sent only</Trans>
										</SelectItem>
									</SelectContent>
								</Select>
							</div>
							{readonly && (
								<p className="rounded-md border bg-muted/40 p-3 text-sm text-muted-foreground">
									<Trans>You have read-only access. Traffic settings cannot be changed.</Trans>
								</p>
							)}
						</div>
						{!readonly && (
							<SheetFooter className="border-t p-6">
								<Button type="submit" disabled={saving || invalidQuota}>
									{saving ? <Trans>Saving...</Trans> : <Trans>Save</Trans>}
								</Button>
							</SheetFooter>
						)}
					</form>
				</SheetContent>
			</Sheet>
			<AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>
							<Trans>Change billing day?</Trans>
						</AlertDialogTitle>
						<AlertDialogDescription>
							<Trans>Changing the billing day may start a new cycle and reset the current traffic usage.</Trans>
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>
							<Trans>Cancel</Trans>
						</AlertDialogCancel>
						<AlertDialogAction onClick={save}>
							<Trans>Change billing day</Trans>
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	)
}
