<<<<<<< HEAD
const browserAPIBase = import.meta.env.PUBLIC_API_URL || "";
=======
const browserAPIBase = import.meta.env.PUBLIC_API_URL || "http://localhost:8080";
>>>>>>> dev-deepu
const serverAPIBase =
    import.meta.env.PRIVATE_API_URL ||
    import.meta.env.PUBLIC_API_URL ||
    "http://127.0.0.1:8080";
const API_BASE = import.meta.env.SSR ? serverAPIBase : browserAPIBase;

async function fetchAPI<T>(endpoint: string, options?: RequestInit): Promise<T> {
    const response = await fetch(`${API_BASE}${endpoint}`, {
        ...(import.meta.env.SSR ? {} : { credentials: "include" as const }),
        ...options,
        headers: {
            "Content-Type": "application/json",
            ...options?.headers,
        },
    });

    if (!response.ok) {
        let detail = `${response.status} ${response.statusText}`;
        try {
            const data = await response.json();
            if (data?.error) {
                detail = data.error;
            }
        } catch {
            // Use the default status text when the body is not JSON.
        }
        throw new Error(detail);
    }

    return response.json();
}

// Job endpoints
export async function fetchJobs(params: Record<string, string | number | undefined> = {}) {
    const searchParams = new URLSearchParams();
    Object.entries(params).forEach(([key, value]) => {
        if (value !== undefined && value !== '') {
            searchParams.set(key, String(value));
        }
    });

    const query = searchParams.toString();
    return fetchAPI<import("../types").JobListResponse>(`/api/v1/jobs${query ? `?${query}` : ""}`);
}

export async function fetchJob(id: string) {
    return fetchAPI<import("../types").Job>(`/api/v1/jobs/${id}`);
}

export async function fetchFilters() {
    return fetchAPI<import("../types").FilterOptions>("/api/v1/filters");
}

// Company endpoints
export async function fetchCompanies(query?: string) {
    const params = query ? `?q=${encodeURIComponent(query)}` : '';
    return fetchAPI<import("../types").CompanyListResponse>(`/api/v1/companies${params}`);
}

export async function fetchCompany(slug: string) {
    return fetchAPI<import("../types").CompanyDetailResponse>(`/api/v1/companies/${slug}`);
}

// Analytics endpoints
export async function fetchTopSkills(limit = 20) {
    return fetchAPI<{ data: import("../types").SkillCount[] }>(`/api/v1/analytics/skills?limit=${limit}`);
}

export async function fetchAnalyticsSummary() {
    return fetchAPI<import("../types").AnalyticsSummary>("/api/v1/analytics/summary");
}

export async function fetchMarketTrends(limit = 10) {
    return fetchAPI<{ data: import("../types").MarketTrend[] }>(`/api/v1/analytics/trends?limit=${limit}`);
}

export async function fetchSourceDistribution() {
    return fetchAPI<{ data: import("../types").SourceDistribution[] }>("/api/v1/analytics/sources");
}

export async function fetchSourceHealth() {
    return fetchAPI<{ data: import("../types").SourceHealth[] }>("/api/v1/analytics/source-health");
}

export async function fetchSalaryStats() {
    return fetchAPI<import("../types").SalaryStats>("/api/v1/analytics/salary");
}

export async function refreshTrends() {
    return fetchAPI<{ message: string }>("/api/v1/analytics/refresh", { method: "POST" });
}

// Auth endpoints
export async function register(email: string, password: string, name: string, rememberMe = true) {
    return fetchAPI<import("../types").AuthResponse>("/api/v1/auth/register", {
        method: "POST",
        body: JSON.stringify({ email, password, name, rememberMe }),
    });
}

export async function login(email: string, password: string, rememberMe = true) {
    return fetchAPI<import("../types").AuthResponse>("/api/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({ email, password, rememberMe }),
    });
}

export async function logout() {
    return fetchAPI<{ message: string }>("/api/v1/auth/logout", {
        method: "POST",
    });
}

export async function fetchSession() {
    return fetchAPI<import("../types").SessionResponse>("/api/v1/auth/session");
}

export async function fetchProfile() {
    return fetchAPI<import("../types").UserProfile>("/api/v1/me");
}

export async function fetchSavedJobs() {
    return fetchAPI<import("../types").SavedJobsResponse>("/api/v1/me/saved-jobs");
}

export async function saveJob(jobId: string) {
    return fetchAPI<{ message: string }>(`/api/v1/me/saved-jobs/${jobId}`, {
        method: "POST",
    });
}

export async function unsaveJob(jobId: string) {
    return fetchAPI<{ message: string }>(`/api/v1/me/saved-jobs/${jobId}`, {
        method: "DELETE",
    });
}

// Application Tracker endpoints
export async function fetchApplications() {
    return fetchAPI<{ data: import("../types").Application[] }>("/api/v1/me/applications");
}

export async function createApplication(data: any) {
    return fetchAPI<import("../types").Application>("/api/v1/me/applications", {
        method: "POST",
        body: JSON.stringify(data),
    });
}

export async function updateApplication(id: string, data: any) {
    return fetchAPI<import("../types").Application>(`/api/v1/me/applications/${id}`, {
        method: "PATCH",
        body: JSON.stringify(data),
    });
}

export async function deleteApplication(id: string) {
    return fetchAPI<void>(`/api/v1/me/applications/${id}`, {
        method: "DELETE",
    });
}
