"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, App, Button, Card, Form, Input, InputNumber, Select, Space, Switch, Table, Tag, Upload } from "antd";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { nanoid } from "nanoid";
import { commerceQueryKeys } from "@/services/api/commerce-query-keys";
import {
    cancelVideoRenderJob,
    createVideoRenderJob,
    defaultVideoOutput,
    downloadVideoRenderJob,
    fetchVideoProject,
    fetchVideoProjectVersions,
    fetchVideoRenderJobs,
    freezeVideoProjectVersion,
    newShot,
    preflightVideoProject,
    reviewVideoRenderJob,
    saveVideoProject,
    type VideoPreflight,
    type VideoProject,
    type VideoTimeline,
    type VideoTimelineShot,
    type VideoTimelineSubtitle,
} from "@/services/api/video-projects";
import { uploadWorkspaceFile } from "@/services/api/workspace";
import { useUserStore } from "@/stores/use-user-store";

function recomputeStarts(shots: VideoTimelineShot[]): VideoTimelineShot[] {
    let cursor = 0;
    return shots.map((shot) => {
        const next = { ...shot, startMs: cursor };
        cursor += shot.durationMs;
        return next;
    });
}

function SafeAreaPreview({ timeline }: { timeline: VideoTimeline }) {
    const { width, height, ratio } = timeline.output;
    const scale = Math.min(360 / width, 640 / height, 1);
    const w = Math.round(width * scale);
    const h = Math.round(height * scale);
    return (
        <div className="flex flex-col items-center gap-2">
            <div className="text-xs text-neutral-500">
                安全区预览 · {ratio} · {width}×{height}
            </div>
            <div className="relative border border-neutral-300 bg-neutral-900" style={{ width: w, height: h }}>
                <div className="pointer-events-none absolute inset-[10%] border border-dashed border-emerald-400/70" />
                <div className="pointer-events-none absolute inset-x-[5%] top-[10%] bottom-[10%] border border-dashed border-amber-300/50" />
                {timeline.subtitles.map((sub) => (
                    <div key={sub.id} className="absolute left-1/2 max-w-[90%] -translate-x-1/2 truncate rounded bg-black/60 px-2 py-0.5 text-center text-[10px] text-white" style={{ top: `${sub.positionY}%` }} title={sub.text}>
                        {sub.text || "字幕"}
                    </div>
                ))}
                <div className="absolute bottom-1 left-1 right-1 flex gap-1 overflow-hidden">
                    {timeline.shots.map((shot) => (
                        <div key={shot.id} className="h-2 flex-1 rounded-sm bg-sky-500/70" title={`${shot.durationMs}ms`} />
                    ))}
                </div>
            </div>
            <div className="text-xs text-neutral-500">绿框=字幕安全区 · 黄框=动作安全区 · 底条=镜头占比</div>
        </div>
    );
}

const PREFLIGHT_CODE_MESSAGES: Record<string, string> = {
    SHOT_COUNT: "镜头数需 3-100",
    TOTAL_DURATION: "总时长需 15-60 秒",
    SHOT_ID: "镜头编号不能为空且不能重复",
    SHOT_DURATION: "镜头时长或裁切方式无效",
    TRANSITION: "镜头转场无效",
    TRANSITION_DURATION: "转场时长不能超过镜头时长",
    SHOT_SOURCE: "镜头素材无效",
    SHOT_RANGE: "镜头开始时间和裁切起点不能为负数",
    SUBTITLE_RANGE: "字幕文本、时间或安全区位置无效",
    BGM_RIGHTS: "背景音乐必须引用企业音频并确认使用权",
    BGM_VOLUME: "背景音乐音量必须在 0 到 100 之间",
    OUTPUT_SPEC: "输出规格必须为支持的 1080p、30fps 工程契约",
    FILE_MISSING: "工程素材不存在或不属于当前企业",
    BGM_MIME: "背景音乐必须是音频文件",
    SHOT_MIME: "镜头素材类型与声明不一致",
};

function preflightIssueMessage(issue: { code?: string; message?: string }) {
    const mapped = issue.code ? PREFLIGHT_CODE_MESSAGES[issue.code] : "";
    const message = (issue.message || "").trim();
    if (message && !/^\[[A-Z0-9_]+\]/.test(message)) return message;
    return mapped || message || issue.code || "预检未通过";
}

export default function VideoProjectEditorPage() {
    const { message } = App.useApp();
    const params = useParams<{ id: string }>();
    const projectId = params.id;
    const queryClient = useQueryClient();
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const token = useUserStore((state) => state.token || "");
    const [name, setName] = useState("");
    const [description, setDescription] = useState("");
    const [timeline, setTimeline] = useState<VideoTimeline | null>(null);
    const [preflight, setPreflight] = useState<VideoPreflight | null>(null);
    const [saving, setSaving] = useState(false);

    const projectQuery = useQuery({
        queryKey: commerceQueryKeys.videoProject(organizationId, projectId),
        queryFn: () => fetchVideoProject(projectId),
        enabled: Boolean(organizationId && projectId),
    });
    const versionsQuery = useQuery({
        queryKey: [...commerceQueryKeys.videoProject(organizationId, projectId), "versions"],
        queryFn: () => fetchVideoProjectVersions(projectId),
        enabled: Boolean(organizationId && projectId),
    });
    const renderFilters = useMemo(() => ({ type: projectId, page: 1, pageSize: 50 }), [projectId]);
    const rendersQuery = useQuery({
        queryKey: commerceQueryKeys.videoRenderJobs(organizationId, renderFilters),
        queryFn: () => fetchVideoRenderJobs(renderFilters),
        enabled: Boolean(organizationId && projectId),
        refetchInterval: 5000,
    });

    useEffect(() => {
        if (!projectQuery.data) return;
        setName(projectQuery.data.name);
        setDescription(projectQuery.data.description || "");
        setTimeline(projectQuery.data.draftTimeline || { shots: [], subtitles: [], output: defaultVideoOutput() });
    }, [projectQuery.data]);

    const durationMs = useMemo(() => {
        if (!timeline) return 0;
        return timeline.shots.reduce((max, shot) => Math.max(max, shot.startMs + shot.durationMs), 0);
    }, [timeline]);

    async function persist(nextTimeline = timeline): Promise<VideoProject | undefined> {
        if (!projectQuery.data || !nextTimeline) return undefined;
        setSaving(true);
        try {
            const saved = await saveVideoProject(projectId, {
                name,
                description,
                productId: projectQuery.data.productId,
                skuId: projectQuery.data.skuId,
                timeline: nextTimeline,
                expectedVersion: projectQuery.data.version,
            });
            queryClient.setQueryData(commerceQueryKeys.videoProject(organizationId, projectId), saved);
            setTimeline(saved.draftTimeline);
            message.success("草稿已保存");
            return saved;
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存失败");
            throw error;
        } finally {
            setSaving(false);
        }
    }

    function updateShots(updater: (shots: VideoTimelineShot[]) => VideoTimelineShot[]) {
        setTimeline((current) => {
            if (!current) return current;
            return { ...current, shots: recomputeStarts(updater(current.shots)) };
        });
    }

    async function uploadShotFile(file: File, kind: "image" | "video") {
        if (!token) {
            message.error("未登录");
            return;
        }
        const storageKey = `${kind}:${nanoid()}`;
        await uploadWorkspaceFile(token, storageKey, file);
        updateShots((shots) => [...shots, newShot({ source: { storageKey, kind, sourceType: "upload" }, durationMs: kind === "image" ? 3000 : 5000 })]);
        message.success("素材已加入时间线");
    }

    async function uploadBgm(file: File) {
        if (!token || !timeline) return;
        const storageKey = `audio:${nanoid()}`;
        await uploadWorkspaceFile(token, storageKey, file);
        setTimeline({
            ...timeline,
            bgm: { storageKey, volume: 40, loop: true, trimStartMs: 0, fadeInMs: 500, fadeOutMs: 500, rightsConfirmed: true },
        });
        message.success("BGM 已设置");
    }

    const runPreflight = useMutation({
        mutationFn: async () => {
            await persist();
            return preflightVideoProject(projectId);
        },
        onSuccess: (result) => {
            setPreflight(result);
            if (result.canFreeze) message.success("预检通过，可冻结版本");
            else message.warning("预检未通过，请查看问题列表");
        },
        onError: (error: Error) => message.error(error.message || "预检失败"),
    });

    const freeze = useMutation({
        mutationFn: async () => {
            if (!projectQuery.data) throw new Error("工程未加载");
            const saved = await persist();
            if (!saved) throw new Error("工程未加载");
            // Use post-save version: persist may bump (real edits) or keep (no-op).
            return freezeVideoProjectVersion(projectId, saved.version);
        },
        onSuccess: async () => {
            message.success("版本已冻结");
            await queryClient.invalidateQueries({ queryKey: commerceQueryKeys.videoProject(organizationId, projectId) });
            await versionsQuery.refetch();
        },
        onError: (error: Error) => message.error(error.message || "冻结失败"),
    });

    const submitRender = useMutation({
        mutationFn: async () => {
            if (!projectQuery.data?.currentVersion) throw new Error("请先冻结版本");
            return createVideoRenderJob(projectId, {
                requestId: `render-${projectId}-v${projectQuery.data.currentVersion}-${nanoid(8)}`,
                expectedVersion: projectQuery.data.version,
                versionNumber: projectQuery.data.currentVersion,
            });
        },
        onSuccess: async () => {
            message.success("渲染任务已提交");
            await rendersQuery.refetch();
        },
        onError: (error: Error) => message.error(error.message || "提交渲染失败"),
    });

    if (projectQuery.isLoading || !timeline) {
        return <Card loading />;
    }

    return (
        <div className="space-y-4">
            <Space>
                <Link href="/commerce/video-projects">← 返回列表</Link>
                <Tag>{projectQuery.data?.status}</Tag>
                <Tag>草稿 v{projectQuery.data?.version}</Tag>
                <Tag>冻结 v{projectQuery.data?.currentVersion || 0}</Tag>
            </Space>
            <Alert type="info" showIcon message="Release 2 编辑器" description="保存草稿 → 预检 → 冻结版本 → 提交渲染。成片由独立 video-worker 用 FFmpeg 生成，审核通过后可下载。" />
            <div className="grid gap-4 lg:grid-cols-[1fr_320px]">
                <div className="space-y-4">
                    <Card
                        title="基本信息"
                        extra={
                            <Button type="primary" loading={saving} onClick={() => persist()}>
                                保存草稿
                            </Button>
                        }
                    >
                        <Form layout="vertical">
                            <Form.Item label="名称">
                                <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={200} />
                            </Form.Item>
                            <Form.Item label="描述">
                                <Input.TextArea value={description} onChange={(e) => setDescription(e.target.value)} rows={2} maxLength={2000} />
                            </Form.Item>
                            <Form.Item label="画幅">
                                <Select
                                    value={timeline.output.ratio}
                                    options={[
                                        { value: "16:9", label: "16:9" },
                                        { value: "9:16", label: "9:16" },
                                        { value: "1:1", label: "1:1" },
                                    ]}
                                    onChange={(ratio) => setTimeline({ ...timeline, output: defaultVideoOutput(ratio) })}
                                />
                            </Form.Item>
                        </Form>
                    </Card>

                    <Card
                        title={`镜头时间线（${timeline.shots.length} 镜 · ${(durationMs / 1000).toFixed(1)}s）`}
                        extra={
                            <Space>
                                <Upload
                                    accept="image/*"
                                    showUploadList={false}
                                    beforeUpload={(file) => {
                                        void uploadShotFile(file, "image");
                                        return false;
                                    }}
                                >
                                    <Button>添加图片镜头</Button>
                                </Upload>
                                <Upload
                                    accept="video/*"
                                    showUploadList={false}
                                    beforeUpload={(file) => {
                                        void uploadShotFile(file, "video");
                                        return false;
                                    }}
                                >
                                    <Button>添加视频镜头</Button>
                                </Upload>
                            </Space>
                        }
                    >
                        <Table
                            rowKey="id"
                            size="small"
                            pagination={false}
                            dataSource={timeline.shots}
                            columns={[
                                { title: "#", width: 48, render: (_: unknown, __: VideoTimelineShot, index: number) => index + 1 },
                                { title: "素材", dataIndex: ["source", "storageKey"], ellipsis: true },
                                { title: "类型", dataIndex: ["source", "kind"], width: 72 },
                                {
                                    title: "时长(ms)",
                                    dataIndex: "durationMs",
                                    width: 120,
                                    render: (value: number, record: VideoTimelineShot) => (
                                        <InputNumber min={200} max={60000} value={value} onChange={(next) => updateShots((shots) => shots.map((shot) => (shot.id === record.id ? { ...shot, durationMs: Number(next) || 200 } : shot)))} />
                                    ),
                                },
                                {
                                    title: "裁切",
                                    dataIndex: "cropMode",
                                    width: 120,
                                    render: (value: string, record: VideoTimelineShot) => (
                                        <Select
                                            value={value}
                                            style={{ width: 100 }}
                                            options={[
                                                { value: "cover", label: "cover" },
                                                { value: "contain", label: "contain" },
                                            ]}
                                            onChange={(value) => {
                                                const cropMode = value as "cover" | "contain";
                                                updateShots((shots) => shots.map((shot) => (shot.id === record.id ? { ...shot, cropMode } : shot)));
                                            }}
                                        />
                                    ),
                                },
                                {
                                    title: "转场",
                                    width: 140,
                                    render: (_: unknown, record: VideoTimelineShot) => (
                                        <Select
                                            value={record.transitionToNext.type}
                                            style={{ width: 120 }}
                                            options={[
                                                { value: "none", label: "无" },
                                                { value: "fade", label: "淡入淡出" },
                                                { value: "cross_dissolve", label: "叠化" },
                                            ]}
                                            onChange={(value) => {
                                                const type = value as "none" | "fade" | "cross_dissolve";
                                                updateShots((shots) => shots.map((shot) => (shot.id === record.id ? { ...shot, transitionToNext: { type, durationMs: type === "none" ? 0 : 400 } } : shot)));
                                            }}
                                        />
                                    ),
                                },
                                {
                                    title: "操作",
                                    width: 160,
                                    render: (_: unknown, record: VideoTimelineShot, index: number) => (
                                        <Space size={4}>
                                            <Button
                                                size="small"
                                                disabled={index === 0}
                                                onClick={() =>
                                                    updateShots((shots) => {
                                                        const next = [...shots];
                                                        [next[index - 1], next[index]] = [next[index], next[index - 1]];
                                                        return next;
                                                    })
                                                }
                                            >
                                                上移
                                            </Button>
                                            <Button
                                                size="small"
                                                disabled={index === timeline.shots.length - 1}
                                                onClick={() =>
                                                    updateShots((shots) => {
                                                        const next = [...shots];
                                                        [next[index], next[index + 1]] = [next[index + 1], next[index]];
                                                        return next;
                                                    })
                                                }
                                            >
                                                下移
                                            </Button>
                                            <Button size="small" danger onClick={() => updateShots((shots) => shots.filter((shot) => shot.id !== record.id))}>
                                                删
                                            </Button>
                                        </Space>
                                    ),
                                },
                            ]}
                        />
                    </Card>

                    <Card
                        title="字幕"
                        extra={
                            <Button
                                onClick={() =>
                                    setTimeline({
                                        ...timeline,
                                        subtitles: [...timeline.subtitles, { id: nanoid(8), text: "新字幕", startMs: 0, endMs: Math.min(2000, Math.max(durationMs, 2000)), style: "default", positionY: 80 }],
                                    })
                                }
                            >
                                添加字幕
                            </Button>
                        }
                    >
                        <Table
                            rowKey="id"
                            size="small"
                            pagination={false}
                            dataSource={timeline.subtitles}
                            columns={[
                                {
                                    title: "文本",
                                    dataIndex: "text",
                                    render: (value: string, record: VideoTimelineSubtitle) => (
                                        <Input value={value} onChange={(e) => setTimeline({ ...timeline, subtitles: timeline.subtitles.map((item) => (item.id === record.id ? { ...item, text: e.target.value } : item)) })} />
                                    ),
                                },
                                {
                                    title: "开始ms",
                                    width: 110,
                                    render: (_: unknown, record: VideoTimelineSubtitle) => (
                                        <InputNumber
                                            value={record.startMs}
                                            min={0}
                                            onChange={(startMs) => setTimeline({ ...timeline, subtitles: timeline.subtitles.map((item) => (item.id === record.id ? { ...item, startMs: Number(startMs) || 0 } : item)) })}
                                        />
                                    ),
                                },
                                {
                                    title: "结束ms",
                                    width: 110,
                                    render: (_: unknown, record: VideoTimelineSubtitle) => (
                                        <InputNumber value={record.endMs} min={1} onChange={(endMs) => setTimeline({ ...timeline, subtitles: timeline.subtitles.map((item) => (item.id === record.id ? { ...item, endMs: Number(endMs) || 1 } : item)) })} />
                                    ),
                                },
                                {
                                    title: "位置Y%",
                                    width: 110,
                                    render: (_: unknown, record: VideoTimelineSubtitle) => (
                                        <InputNumber
                                            value={record.positionY}
                                            min={10}
                                            max={90}
                                            onChange={(positionY) => setTimeline({ ...timeline, subtitles: timeline.subtitles.map((item) => (item.id === record.id ? { ...item, positionY: Number(positionY) || 80 } : item)) })}
                                        />
                                    ),
                                },
                                {
                                    title: "",
                                    width: 64,
                                    render: (_: unknown, record: VideoTimelineSubtitle) => (
                                        <Button size="small" danger onClick={() => setTimeline({ ...timeline, subtitles: timeline.subtitles.filter((item) => item.id !== record.id) })}>
                                            删
                                        </Button>
                                    ),
                                },
                            ]}
                        />
                    </Card>

                    <Card
                        title="背景音乐"
                        extra={
                            <Upload
                                accept="audio/*"
                                showUploadList={false}
                                beforeUpload={(file) => {
                                    void uploadBgm(file);
                                    return false;
                                }}
                            >
                                <Button>上传 BGM</Button>
                            </Upload>
                        }
                    >
                        {timeline.bgm ? (
                            <Space wrap>
                                <Tag>{timeline.bgm.storageKey}</Tag>
                                <span>音量</span>
                                <InputNumber min={0} max={100} value={timeline.bgm.volume} onChange={(volume) => setTimeline({ ...timeline, bgm: { ...timeline.bgm!, volume: Number(volume) || 0 } })} />
                                <span>确认版权</span>
                                <Switch checked={timeline.bgm.rightsConfirmed} onChange={(rightsConfirmed) => setTimeline({ ...timeline, bgm: { ...timeline.bgm!, rightsConfirmed } })} />
                                <Button danger onClick={() => setTimeline({ ...timeline, bgm: undefined })}>
                                    移除
                                </Button>
                            </Space>
                        ) : (
                            <div className="text-neutral-500">未设置 BGM</div>
                        )}
                    </Card>
                </div>

                <div className="space-y-4">
                    <Card title="安全区 / 画幅预览">
                        <SafeAreaPreview timeline={timeline} />
                    </Card>
                    <Card title="发布与渲染">
                        <Space direction="vertical" className="w-full">
                            <Button block loading={runPreflight.isPending} onClick={() => runPreflight.mutate()}>
                                预检
                            </Button>
                            <Button block loading={freeze.isPending} onClick={() => freeze.mutate()}>
                                冻结版本
                            </Button>
                            <Button block type="primary" loading={submitRender.isPending} onClick={() => submitRender.mutate()}>
                                提交渲染
                            </Button>
                            {preflight && (
                                <div className="text-xs">
                                    <div>
                                        时长 {(preflight.durationMs / 1000).toFixed(1)}s · {preflight.canFreeze ? "可冻结" : "不可冻结"}
                                    </div>
                                    {preflight.issues.map((issue) => (
                                        <div key={issue.code + issue.message} className={issue.severity === "error" ? "text-red-600" : "text-amber-600"}>
                                            {preflightIssueMessage(issue)}
                                        </div>
                                    ))}
                                </div>
                            )}
                            <div className="text-xs text-neutral-500">已冻结版本：{(versionsQuery.data || []).map((item) => `v${item.version}`).join(", ") || "无"}</div>
                        </Space>
                    </Card>
                    <Card title="渲染任务">
                        <Table
                            rowKey="id"
                            size="small"
                            pagination={false}
                            loading={rendersQuery.isLoading}
                            dataSource={rendersQuery.data?.items || []}
                            columns={[
                                {
                                    title: "状态",
                                    dataIndex: "status",
                                    width: 110,
                                    render: (value: string, record) => (
                                        <Tag>
                                            {value}/{record.phase}
                                        </Tag>
                                    ),
                                },
                                { title: "算力", dataIndex: "estimatedCredits", width: 64 },
                                {
                                    title: "操作",
                                    render: (_: unknown, record) => (
                                        <Space size={4} wrap>
                                            {["queued", "materializing", "rendering", "archiving"].includes(record.status) && (
                                                <Button
                                                    size="small"
                                                    onClick={async () => {
                                                        await cancelVideoRenderJob(record.id);
                                                        await rendersQuery.refetch();
                                                    }}
                                                >
                                                    取消
                                                </Button>
                                            )}
                                            {record.status === "pending_review" && (
                                                <>
                                                    <Button
                                                        size="small"
                                                        type="primary"
                                                        onClick={async () => {
                                                            await reviewVideoRenderJob(record.id, "approved");
                                                            await rendersQuery.refetch();
                                                        }}
                                                    >
                                                        通过
                                                    </Button>
                                                    <Button
                                                        size="small"
                                                        danger
                                                        onClick={async () => {
                                                            await reviewVideoRenderJob(record.id, "rejected");
                                                            await rendersQuery.refetch();
                                                        }}
                                                    >
                                                        驳回
                                                    </Button>
                                                </>
                                            )}
                                            {["pending_review", "completed"].includes(record.status) && record.resultStorageKey && (
                                                <Button
                                                    size="small"
                                                    onClick={async () => {
                                                        const result = await downloadVideoRenderJob(record.id);
                                                        window.open(result.url, "_blank");
                                                    }}
                                                >
                                                    下载
                                                </Button>
                                            )}
                                        </Space>
                                    ),
                                },
                            ]}
                        />
                    </Card>
                </div>
            </div>
        </div>
    );
}
