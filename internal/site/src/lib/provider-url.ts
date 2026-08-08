export function normalizeProviderURL(value: string): string {
	const trimmed = value.trim()
	return trimmed && !/^https?:\/\//i.test(trimmed) ? `https://${trimmed}` : trimmed
}

export function getProviderWebsiteURL(value?: string): string | null {
	if (!value) return null
	try {
		const url = new URL(value)
		return url.protocol === "http:" || url.protocol === "https:" ? url.href : null
	} catch {
		return null
	}
}
