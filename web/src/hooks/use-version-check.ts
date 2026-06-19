import { useCallback, useEffect, useMemo, useState } from "react";
import { App } from "antd";
import { APP_VERSION } from "@/constant/env";
import type { ReleaseInfo } from "@/lib/release";

const remoteVersionUrl = "https://raw.githubusercontent.com/yypyyd/infinite-canvas/main/VERSION";
const releaseUrl = "/api/app/releases";

function readLocalReleases(): ReleaseInfo[] {
    try {
        return JSON.parse(process.env.NEXT_PUBLIC_APP_RELEASES || "[]");
    } catch {
        return [];
    }
}

function toVersionParts(version: string) {
    const match = version.trim().match(/^v?(\d+)\.(\d+)\.(\d+)/);
    return match ? match.slice(1).map(Number) : null;
}

function isNewerVersion(latestVersion: string, currentVersion: string) {
    const latest = toVersionParts(latestVersion);
    const current = toVersionParts(currentVersion);
    if (!latest || !current) return false;
    return latest.some((value, index) => value > current[index] && latest.slice(0, index).every((part, prevIndex) => part === current[prevIndex]));
}

function newerOrCurrent(remoteVersion: string, currentVersion: string) {
    const version = remoteVersion.trim();
    return isNewerVersion(version, currentVersion) ? version : currentVersion;
}

async function fetchReleaseInfo() {
    const response = await fetch(releaseUrl, { cache: "no-store" });
    if (!response.ok) throw new Error("版本信息读取失败");
    return (await response.json()) as { version?: string; releases?: ReleaseInfo[] };
}

export function useVersionCheck() {
    const currentVersion = APP_VERSION;
    const { message } = App.useApp();
    const localReleases = useMemo(readLocalReleases, []);
    const [latestVersion, setLatestVersion] = useState(currentVersion);
    const [releases, setReleases] = useState<ReleaseInfo[]>(localReleases);
    const [checking, setChecking] = useState(false);
    const [open, setOpen] = useState(false);
    const hasNewVersion = isNewerVersion(latestVersion, currentVersion);

    const checkLatestVersion = useCallback(async () => {
        try {
            const response = await fetch(remoteVersionUrl, { cache: "no-store" });
            if (!response.ok) return false;
            const version = await response.text();
            setLatestVersion(newerOrCurrent(version, currentVersion));
            return true;
        } catch {
            return false;
        }
    }, [currentVersion]);

    const checkLatestRelease = useCallback(
        async (showMessage = false) => {
            setChecking(true);
            setReleases(localReleases);
            try {
                const [releaseInfo, versionResponse] = await Promise.all([fetchReleaseInfo(), fetch(remoteVersionUrl, { cache: "no-store" })]);
                if (!versionResponse.ok) throw new Error("远程版本读取失败");
                const remoteVersion = await versionResponse.text();
                setLatestVersion(newerOrCurrent(remoteVersion || releaseInfo.version || "", currentVersion));
                if (releaseInfo.releases?.length) setReleases(releaseInfo.releases);
                if (showMessage) message.success("已获取自有仓库版本信息");
                return true;
            } catch {
                setLatestVersion(currentVersion);
                setReleases(localReleases);
                if (showMessage) message.error("获取自有仓库版本信息失败");
                return false;
            } finally {
                setChecking(false);
            }
        },
        [currentVersion, localReleases, message],
    );

    useEffect(() => {
        void checkLatestVersion();
    }, [checkLatestVersion]);

    const openReleaseModal = useCallback(() => {
        setOpen(true);
        void checkLatestRelease();
    }, [checkLatestRelease]);

    return {
        open,
        setOpen,
        openReleaseModal,
        latestVersion,
        releases,
        checking,
        hasNewVersion,
        checkLatestRelease,
    };
}
