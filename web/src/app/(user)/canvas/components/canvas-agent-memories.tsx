"use client";

import { useCallback, useEffect, useState } from "react";
import { Button, Input, Modal, Pagination, Popconfirm } from "antd";
import { listAgentMemories, updateAgentMemory, type AgentMemory } from "@/services/api/agent";

export function CanvasAgentMemories({ projectId }: { projectId: string }) {
    const [open, setOpen] = useState(false);
    const [page, setPage] = useState(1);
    const [data, setData] = useState<{ items: AgentMemory[]; total: number }>({ items: [], total: 0 });
    const [editing, setEditing] = useState<string>();
    const [content, setContent] = useState("");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const load = useCallback(() => listAgentMemories(projectId, page), [projectId, page]);
    useEffect(() => {
        if (!open) return;
        let disposed = false;
        setData({ items: [], total: 0 });
        setBusy(true);
        setError("");
        load()
            .then((result) => {
                if (!disposed) setData(result);
            })
            .catch((error) => {
                if (!disposed) setError(error instanceof Error ? error.message : "记忆加载失败");
            })
            .finally(() => {
                if (!disposed) setBusy(false);
            });
        return () => {
            disposed = true;
        };
    }, [open, load]);
    const save = async (item: AgentMemory, forget = false) => {
        setBusy(true);
        setError("");
        try {
            await updateAgentMemory(item.id, forget ? { forget: true } : { content });
            setEditing(undefined);
            const next = await load();
            if (page > 1 && !next.items.length) setPage(page - 1);
            else setData(next);
        } catch (error) {
            setError(error instanceof Error ? error.message : "记忆更新失败");
        } finally {
            setBusy(false);
        }
    };
    return (
        <>
            <Button type="text" onClick={() => setOpen(true)}>
                管理长期记忆
            </Button>
            <Modal
                title="长期记忆"
                open={open}
                onCancel={() => {
                    if (!busy) {
                        setOpen(false);
                        setEditing(undefined);
                    }
                }}
                footer={null}
                centered
            >
                <div data-canvas-no-zoom className="space-y-4">
                    <p className="text-sm opacity-60">查看本画布和账户记忆。修改后用于后续规划；已发出的操作不会因此改变。</p>
                    {error ? (
                        <p role="alert" className="text-sm">
                            {error}
                        </p>
                    ) : null}
                    {!busy && !data.items.length ? <p className="py-4 text-sm opacity-60">暂无长期记忆</p> : null}
                    <div className="max-h-96 space-y-4 overflow-auto">
                        {data.items.map((item) => (
                            <div key={item.id} className="space-y-2">
                                <div className="flex items-center justify-between gap-2 text-xs opacity-60">
                                    <span>
                                        {item.projectId ? "本画布" : "账户"} · {item.key}
                                    </span>
                                    {item.expiresAt ? <span>到期：{new Date(item.expiresAt).toLocaleDateString()}</span> : null}
                                </div>
                                {editing === item.id ? (
                                    <Input.TextArea value={content} onChange={(event) => setContent(event.target.value)} maxLength={1000} autoSize={{ minRows: 2, maxRows: 6 }} disabled={busy} />
                                ) : (
                                    <p className="whitespace-pre-wrap text-sm leading-6">{item.content}</p>
                                )}
                                <div className="flex justify-end gap-1">
                                    {editing === item.id ? (
                                        <>
                                            <Button size="small" type="text" disabled={busy} onClick={() => setEditing(undefined)}>
                                                取消
                                            </Button>
                                            <Button size="small" type="primary" loading={busy} disabled={!content.trim()} onClick={() => void save(item)}>
                                                保存
                                            </Button>
                                        </>
                                    ) : (
                                        <Button
                                            size="small"
                                            type="text"
                                            disabled={busy}
                                            onClick={() => {
                                                setEditing(item.id);
                                                setContent(item.content);
                                            }}
                                        >
                                            编辑
                                        </Button>
                                    )}
                                    <Popconfirm title="遗忘这条记忆？" okText="遗忘" cancelText="取消" onConfirm={() => save(item, true)} disabled={busy}>
                                        <Button size="small" type="text" danger disabled={busy}>
                                            遗忘
                                        </Button>
                                    </Popconfirm>
                                </div>
                            </div>
                        ))}
                    </div>
                    {data.total > 20 ? (
                        <Pagination
                            size="small"
                            current={page}
                            pageSize={20}
                            total={data.total}
                            showSizeChanger={false}
                            disabled={busy}
                            onChange={(value) => {
                                setPage(value);
                                setEditing(undefined);
                            }}
                        />
                    ) : null}
                </div>
            </Modal>
        </>
    );
}
