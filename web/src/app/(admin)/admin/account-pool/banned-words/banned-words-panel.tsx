"use client";

import { DeleteOutlined, ImportOutlined, PlusOutlined } from "@ant-design/icons";
import { Button, Card, Input, Modal, Popconfirm, Space, Table, Tabs, Typography } from "antd";
import { useState } from "react";

import { gatewayApi, type GatewayBannedHit, type GatewayBannedWord } from "@/services/api/gateway";
import { formatTime, useGatewayAction, useGatewayQuery } from "../use-gateway";

export default function ContentSafetyPanel() {
    const run = useGatewayAction();
    const [tab, setTab] = useState("words");
    const [page, setPage] = useState(1);
    const [limit, setLimit] = useState(20);
    const [keyword, setKeyword] = useState("");
    const [word, setWord] = useState("");
    const [importOpen, setImportOpen] = useState(false);
    const [importText, setImportText] = useState("");
    const params = { q: keyword, limit, offset: (page - 1) * limit };
    const words = useGatewayQuery(["banned-words", params], (token) => gatewayApi.bannedWords(token, params));
    const hits = useGatewayQuery(["banned-hits", params], (token) => gatewayApi.bannedHits(token, params));
    const pagination = (total: number) => ({ current: page, pageSize: limit, total, showSizeChanger: true, onChange: (next: number, size: number) => (size !== limit ? (setLimit(size), setPage(1)) : setPage(next)) });

    const addWord = async () => {
        if (!word.trim()) return;
        if (await run((token) => gatewayApi.addBannedWord(token, word.trim()), "违禁词已添加")) setWord("");
    };
    const submitImport = async () => {
        const result = await run((token) => gatewayApi.importBannedWords(token, importText));
        if (result) {
            Modal.success({ title: "导入完成", content: `新增 ${result.added} 个，跳过 ${result.skipped} 个` });
            setImportOpen(false);
            setImportText("");
        }
    };

    return (
        <main>
            <Card variant="borderless">
                <Tabs
                    activeKey={tab}
                    onChange={(key) => (setTab(key), setPage(1))}
                    items={[
                        { key: "words", label: "违禁词" },
                        { key: "hits", label: "触发记录" },
                    ]}
                />
                <Space wrap style={{ marginBottom: 16 }}>
                    <Input.Search allowClear placeholder={tab === "words" ? "搜索违禁词" : "搜索违禁词 / 用户 / 提示词"} onSearch={(value) => (setKeyword(value), setPage(1))} style={{ width: 260 }} />
                    {tab === "words" ? (
                        <>
                            <Input value={word} onChange={(event) => setWord(event.target.value)} onPressEnter={() => void addWord()} placeholder="新增违禁词" style={{ width: 200 }} maxLength={255} />
                            <Button type="primary" icon={<PlusOutlined />} onClick={() => void addWord()}>
                                添加
                            </Button>
                            <Button icon={<ImportOutlined />} onClick={() => setImportOpen(true)}>
                                批量导入
                            </Button>
                        </>
                    ) : null}
                </Space>
                {tab === "words" ? (
                    <Table<GatewayBannedWord>
                        rowKey="id"
                        loading={words.isLoading}
                        dataSource={words.data?.data || []}
                        pagination={pagination(words.data?.total || 0)}
                        columns={[
                            { title: "违禁词", dataIndex: "word" },
                            { title: "命中次数", dataIndex: "hits", width: 120 },
                            { title: "添加时间", width: 180, render: (_, item) => formatTime(item.created_at) },
                            {
                                title: "操作",
                                width: 90,
                                render: (_, item) => (
                                    <Popconfirm title={`删除「${item.word}」？`} onConfirm={() => void run((token) => gatewayApi.deleteBannedWord(token, item.id), "已删除")}>
                                        <Button size="small" danger icon={<DeleteOutlined />} />
                                    </Popconfirm>
                                ),
                            },
                        ]}
                    />
                ) : (
                    <Table<GatewayBannedHit>
                        rowKey="id"
                        loading={hits.isLoading}
                        dataSource={hits.data?.data || []}
                        pagination={pagination(hits.data?.total || 0)}
                        columns={[
                            { title: "时间", width: 180, render: (_, item) => formatTime(item.created_at) },
                            { title: "违禁词", dataIndex: "word", width: 160 },
                            { title: "调用方", width: 200, render: (_, item) => item.user_name || item.user_id || "—" },
                            { title: "提示词", render: (_, item) => <Typography.Text ellipsis={{ tooltip: item.prompt }}>{item.prompt}</Typography.Text> },
                        ]}
                    />
                )}
            </Card>
            <Modal open={importOpen} title="批量导入违禁词" okText="导入" cancelText="取消" onOk={() => void submitImport()} onCancel={() => setImportOpen(false)} destroyOnHidden>
                <Typography.Text type="secondary">支持换行、逗号、顿号、分号分隔，已存在的会自动跳过。</Typography.Text>
                <Input.TextArea rows={10} value={importText} onChange={(event) => setImportText(event.target.value)} style={{ marginTop: 8 }} />
            </Modal>
        </main>
    );
}
