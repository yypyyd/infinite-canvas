"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Alert, App, Avatar, Button, Card, Descriptions, Drawer, Empty, Form, Input, InputNumber, Modal, Pagination, Popconfirm, Progress, Select, Statistic, Table, Tabs, Tag, Upload } from "antd";
import { Boxes, Building2, ClipboardList, Columns3, Download, FileStack, History, PackagePlus, Palette, Plus, RefreshCw, ScanText, Settings2, Trash2, UserPlus, Users, WalletCards } from "lucide-react";
import dynamic from "next/dynamic";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { commercePresets } from "@/constant/commerce-presets";
import {
    acceptOrganizationInvitation,
    cancelBatchProductionJob,
    createBatchProductionJob,
    createOrganization,
    deleteBrand,
    deleteProduct,
    deleteProductSKU,
    downloadBatchProductionArchive,
    fetchAuditLogs,
    fetchBatchProductionItems,
    fetchBatchProductionJobs,
    fetchBrands,
    fetchCommerceWorkspace,
    fetchCurrentOrganizationInvitations,
    fetchOrganizationMembers,
    fetchPendingInvitations,
    fetchProducts,
    fetchProductSKUs,
    fetchProductionDeliverySpecs,
    fetchProductionTemplates,
    fetchProductionTemplateVersions,
    inviteOrganizationMember,
    removeOrganizationMember,
    previewProductionPrompt,
    reviewBatchProductionItem,
    retryBatchProductionItem,
    retryBatchProductionJob,
    revokeOrganizationInvitation,
    saveBrand,
    saveProduct,
    saveProductSKU,
    switchOrganization,
    setBatchProductionItemPrimary,
    transferOrganizationOwnership,
    transferOrganizationCredits,
    updateOrganization,
    updateOrganizationCreditSettings,
    updateOrganizationMember,
    type BatchProductionJob,
    type BatchProductionItem,
    type Brand,
    type OrganizationMember,
    type OrganizationCreditMode,
    type OrganizationRole,
    type Product,
    type ProductSKU,
    type ProductionTemplate,
    type ProductionTemplateVersion,
} from "@/services/api/commerce";
import { commerceQueryKeys, userCommerceQueryKeys } from "@/services/api/commerce-query-keys";
import { useUserStore } from "@/stores/use-user-store";
import { useAssetStore } from "@/stores/use-asset-store";
import { flushActiveWorkspaceChanges } from "@/components/layout/workspace-provider";
import { uploadWorkspaceFile, workspaceFileUrl } from "@/services/api/workspace";
import { CanvasNodeType } from "@/app/(user)/canvas/types";

const BatchResultComparison = dynamic(() => import("./components/batch-result-comparison").then((module) => module.BatchResultComparison), { ssr: false });

const roleOptions = [
    { value: "admin", label: "管理员" },
    { value: "member", label: "成员" },
    { value: "reviewer", label: "审核人" },
];
const roleLabels: Record<string, string> = { owner: "所有者", admin: "管理员", member: "成员", reviewer: "审核人" };
const statusLabels: Record<string, string> = { draft: "草稿", active: "在售", paused: "暂停", queued: "排队中", running: "生产中", pending_review: "待审核", partial_success: "部分成功", completed: "已完成", failed: "失败", cancelled: "已取消" };
const statusColors: Record<string, string> = { active: "green", completed: "green", running: "primary", queued: "primary", pending_review: "primary", partial_success: "primary", failed: "red", cancelled: "default", paused: "primary" };
const reviewLabels: Record<string, string> = { pending: "待审核", approved: "已通过", rejected: "已驳回" };
const reviewColors: Record<string, string> = { pending: "primary", approved: "green", rejected: "red" };
const canSetBatchItemPrimary = (item: BatchProductionItem) => item.status === "completed" && item.reviewStatus === "approved" && Boolean(item.resultStorageKey) && !item.isPrimary && Number.isInteger(item.runNumber) && item.runNumber > 0;
type ListQuery = { page: number; pageSize: number; keyword: string };
const initialListQuery: ListQuery = { page: 1, pageSize: 20, keyword: "" };

export default function CommercePage() {
    const { message } = App.useApp();
    const router = useRouter();
    const queryClient = useQueryClient();
    const refreshUser = useUserStore((state) => state.refreshUser);
    const token = useUserStore((state) => state.token);
    const userId = useUserStore((state) => state.user?.id || "");
    const setOrganizationId = useUserStore((state) => state.setOrganizationId);
    const organizationId = useUserStore((state) => state.user?.organizationId || "");
    const [working, setWorking] = useState(false);
    const [activeTab, setActiveTab] = useState("organization");
    const [organizationMode, setOrganizationMode] = useState<"create" | "edit" | null>(null);
    const [creditSettingsOpen, setCreditSettingsOpen] = useState(false);
    const [creditTransferOpen, setCreditTransferOpen] = useState(false);
    const [inviteOpen, setInviteOpen] = useState(false);
    const [brandDraft, setBrandDraft] = useState<Partial<Brand> | null>(null);
    const [productDraft, setProductDraft] = useState<Partial<Product> | null>(null);
    const [selectedProduct, setSelectedProduct] = useState<Product | null>(null);
    const [skuDraft, setSkuDraft] = useState<Partial<ProductSKU> | null>(null);
    const [batchOpen, setBatchOpen] = useState(false);
    const [selectedBatch, setSelectedBatch] = useState<BatchProductionJob | null>(null);
    const [selectedTemplate, setSelectedTemplate] = useState<ProductionTemplate | null>(null);
    const [promptPreview, setPromptPreview] = useState("");
    const [reviewDraft, setReviewDraft] = useState<{ item: BatchProductionItem; status: "approved" | "rejected" } | null>(null);
    const [comparisonItems, setComparisonItems] = useState<BatchProductionItem[]>([]);
    const [comparisonOpen, setComparisonOpen] = useState(false);
    const [brandListQuery, setBrandListQuery] = useState(initialListQuery);
    const [memberListQuery, setMemberListQuery] = useState(initialListQuery);
    const [productListQuery, setProductListQuery] = useState(initialListQuery);
    const [jobListQuery, setJobListQuery] = useState(initialListQuery);
    const [auditListQuery, setAuditListQuery] = useState(initialListQuery);
    const [skuListQuery, setSKUListQuery] = useState(initialListQuery);
    const [itemListQuery, setItemListQuery] = useState(initialListQuery);
    const [itemStatus, setItemStatus] = useState("all");
    const [productOptionKeyword, setProductOptionKeyword] = useState("");
    const [brandOptionKeyword, setBrandOptionKeyword] = useState("");
    const [organizationForm] = Form.useForm<{ name: string }>();
    const [creditSettingsForm] = Form.useForm<{ mode: OrganizationCreditMode; monthlyBudget: number; alertThreshold: number }>();
    const [creditTransferForm] = Form.useForm<{ amount: number }>();
    const [inviteForm] = Form.useForm<{ email: string; role: OrganizationRole }>();
    const [brandForm] = Form.useForm<Partial<Brand>>();
    const [productForm] = Form.useForm<Partial<Product>>();
    const [skuForm] = Form.useForm<Partial<ProductSKU> & { attributesText?: string }>();
    const [batchForm] = Form.useForm<{ name: string; brandId?: string; presetId: string; presetVersion: number; deliverySpecId: string; productIds: string[]; previewSkuId?: string }>();
    const [reviewForm] = Form.useForm<{ comment: string }>();
    const brandLogoUploadInFlight = useRef(false);
    const brandLogoUploadSession = useRef(0);
    const brandLogoUploadAbort = useRef<AbortController | null>(null);
    const brandUploadFileSessions = useRef(new WeakMap<object, number>());
    const skuUploadsInFlightRef = useRef(0);
    const skuUploadSession = useRef(0);
    const skuUploadControllers = useRef(new Set<AbortController>());
    const skuUploadFileSessions = useRef(new WeakMap<object, number>());
    const batchRequestId = useRef("");
    const selectedBatchIdRef = useRef("");
    selectedBatchIdRef.current = selectedBatch?.id || "";
    const [brandLogoUploading, setBrandLogoUploading] = useState(false);
    const [skuUploadsInFlight, setSKUUploadsInFlight] = useState(0);
    const brandLogoStorageKey = Form.useWatch("logoStorageKey", brandForm);
    const skuImageStorageKeys = Form.useWatch("imageStorageKeys", skuForm) || [];
    const batchProductIDs = Form.useWatch("productIds", batchForm) || [];

    useEffect(() => {
        setMemberListQuery(initialListQuery);
        setBrandListQuery(initialListQuery);
        setProductListQuery(initialListQuery);
        setJobListQuery(initialListQuery);
        setAuditListQuery(initialListQuery);
        setSKUListQuery(initialListQuery);
        setItemListQuery(initialListQuery);
        setItemStatus("all");
        setSelectedProduct(null);
        setSelectedBatch(null);
        brandLogoUploadAbort.current?.abort();
        brandLogoUploadSession.current += 1;
        brandLogoUploadAbort.current = null;
        brandLogoUploadInFlight.current = false;
        setBrandLogoUploading(false);
        skuUploadSession.current += 1;
        skuUploadControllers.current.forEach((controller) => controller.abort());
        skuUploadControllers.current.clear();
        skuUploadsInFlightRef.current = 0;
        setSKUUploadsInFlight(0);
        setBrandDraft(null);
        setProductDraft(null);
        setSkuDraft(null);
        setBatchOpen(false);
        setReviewDraft(null);
        setComparisonItems([]);
        setComparisonOpen(false);
        setSelectedTemplate(null);
        setPromptPreview("");
    }, [organizationId]);

    const workspaceQuery = useQuery({ queryKey: commerceQueryKeys.workspace(organizationId), queryFn: fetchCommerceWorkspace, enabled: Boolean(organizationId) });
    const membersQuery = useQuery({ queryKey: commerceQueryKeys.members(organizationId, memberListQuery), queryFn: () => fetchOrganizationMembers(memberListQuery), enabled: Boolean(organizationId) && activeTab === "organization" });
    const brandsQuery = useQuery({ queryKey: commerceQueryKeys.brands(organizationId, brandListQuery), queryFn: () => fetchBrands(brandListQuery), enabled: Boolean(organizationId) && activeTab === "brands" });
    const productsQuery = useQuery({ queryKey: commerceQueryKeys.products(organizationId, productListQuery), queryFn: () => fetchProducts(productListQuery), enabled: Boolean(organizationId) && activeTab === "products" });
    const jobsQuery = useQuery({
        queryKey: commerceQueryKeys.jobs(organizationId, jobListQuery),
        queryFn: () => fetchBatchProductionJobs(jobListQuery),
        enabled: Boolean(organizationId) && activeTab === "batch",
        refetchInterval: (query) => ((query.state.data as { items?: BatchProductionJob[] } | undefined)?.items?.some((item) => item.status === "queued" || item.status === "running") ? 5000 : false),
    });
    const invitationsQuery = useQuery({ queryKey: userCommerceQueryKeys.pendingInvitations(userId), queryFn: fetchPendingInvitations, enabled: Boolean(userId) && activeTab === "organization" });
    const organizationInvitationsQuery = useQuery({
        queryKey: commerceQueryKeys.organizationInvitations(organizationId),
        queryFn: fetchCurrentOrganizationInvitations,
        enabled: Boolean(organizationId) && activeTab === "organization" && ["owner", "admin"].includes(workspaceQuery.data?.membership.role || ""),
    });
    const auditQuery = useQuery({
        queryKey: commerceQueryKeys.audit(organizationId, auditListQuery),
        queryFn: () => fetchAuditLogs(auditListQuery),
        enabled: Boolean(organizationId) && activeTab === "audit" && ["owner", "admin"].includes(workspaceQuery.data?.membership.role || ""),
    });
    const skusQuery = useQuery({
        queryKey: commerceQueryKeys.skus(organizationId, selectedProduct?.id || "", skuListQuery),
        queryFn: () => fetchProductSKUs(selectedProduct?.id || "", skuListQuery),
        enabled: activeTab === "products" && Boolean(selectedProduct?.id),
    });
    const itemFilters = { ...itemListQuery, type: itemStatus };
    const batchItemsQuery = useQuery({
        queryKey: commerceQueryKeys.items(organizationId, selectedBatch?.id || "", itemFilters),
        queryFn: () => fetchBatchProductionItems(selectedBatch?.id || "", itemFilters),
        enabled: activeTab === "batch" && Boolean(selectedBatch?.id),
        refetchInterval: (query) => ((query.state.data as { items?: Array<{ status: string }> } | undefined)?.items?.some((item) => item.status === "queued" || item.status === "running") ? 5000 : false),
    });
    const templatesQuery = useQuery({
        queryKey: commerceQueryKeys.templates(organizationId, { page: 1, pageSize: 100 }),
        queryFn: () => fetchProductionTemplates({ page: 1, pageSize: 100 }),
        enabled: Boolean(organizationId) && (activeTab === "templates" || activeTab === "batch" || batchOpen),
    });
    const deliverySpecsQuery = useQuery({ queryKey: commerceQueryKeys.deliverySpecs(organizationId), queryFn: fetchProductionDeliverySpecs, enabled: Boolean(organizationId) && (activeTab === "batch" || batchOpen) });
    const templateVersionsQuery = useQuery({
        queryKey: commerceQueryKeys.templateVersions(organizationId, selectedTemplate?.id || ""),
        queryFn: () => fetchProductionTemplateVersions(selectedTemplate?.id || ""),
        enabled: Boolean(organizationId && selectedTemplate?.id),
    });
    const previewSKUsQuery = useQuery({
        queryKey: commerceQueryKeys.previewSkus(organizationId, batchProductIDs[0] || "", { page: 1, pageSize: 100 }),
        queryFn: () => fetchProductSKUs(batchProductIDs[0], { page: 1, pageSize: 100 }),
        enabled: Boolean(organizationId) && batchOpen && Boolean(batchProductIDs[0]),
    });
    const productOptionsQuery = useQuery({
        queryKey: commerceQueryKeys.productOptions(organizationId, { page: 1, pageSize: 50, keyword: productOptionKeyword }),
        queryFn: () => fetchProducts({ page: 1, pageSize: 50, keyword: productOptionKeyword }),
        enabled: Boolean(organizationId) && batchOpen,
    });
    const brandOptionsQuery = useQuery({
        queryKey: commerceQueryKeys.brandOptions(organizationId, { page: 1, pageSize: 50, keyword: brandOptionKeyword }),
        queryFn: () => fetchBrands({ page: 1, pageSize: 50, keyword: brandOptionKeyword }),
        enabled: Boolean(organizationId) && Boolean(productDraft || batchOpen),
    });

    const workspace = workspaceQuery.data;
    const brands = brandsQuery.data?.items || [];
    const products = productsQuery.data?.items || [];
    const jobs = jobsQuery.data?.items || [];
    const productionTemplates = templatesQuery.data?.items || [];
    const productionTemplateOptions = [
        ...commercePresets.map((item) => ({ id: item.id, title: item.title, description: item.description, version: 1 })),
        ...productionTemplates.filter((item) => item.status === "active").map((item) => ({ id: item.id, title: item.name, description: item.description || `企业模板 v${item.currentVersion}`, version: item.currentVersion })),
    ];
    const brandOptions = brandOptionsQuery.data?.items || brands;
    const canManage = ["owner", "admin"].includes(workspace?.membership.role || "");
    const canWrite = canManage || workspace?.membership.role === "member";
    const canReview = canManage || workspace?.membership.role === "reviewer";
    const creditSummary = workspace?.creditSummary;
    const budgetPercent = creditSummary?.monthlyBudget ? Math.min(100, Math.round((creditSummary.monthlyUsed * 100) / creditSummary.monthlyBudget)) : 0;
    const invalidate = async () => {
        if (!organizationId) return;
        await queryClient.invalidateQueries({ queryKey: commerceQueryKeys.root(organizationId), exact: false });
    };
    const run = async (action: () => Promise<unknown>, success: string, invalidateAfter = true) => {
        setWorking(true);
        try {
            await action();
            if (invalidateAfter) await invalidate();
            message.success(success);
            return true;
        } catch (error) {
            message.error(error instanceof Error ? error.message : "操作失败");
            return false;
        } finally {
            setWorking(false);
        }
    };
    const changeOrganizationContext = async (changeOrganization: () => Promise<string>) => {
        const previousOrganizationId = organizationId;
        await flushActiveWorkspaceChanges();
        if (previousOrganizationId) await queryClient.cancelQueries({ queryKey: commerceQueryKeys.root(previousOrganizationId), exact: false });
        const nextOrganizationId = await changeOrganization();
        if (previousOrganizationId && previousOrganizationId !== nextOrganizationId) queryClient.removeQueries({ queryKey: commerceQueryKeys.root(previousOrganizationId), exact: false });
        setOrganizationId(nextOrganizationId);
        await refreshUser();
        await queryClient.invalidateQueries({ queryKey: commerceQueryKeys.root(nextOrganizationId), exact: false });
        await queryClient.refetchQueries({ queryKey: commerceQueryKeys.root(nextOrganizationId), exact: false, type: "active" });
    };

    const canReviewBatchItem = (item: BatchProductionItem, status?: "approved" | "rejected") =>
        canReview && Boolean(selectedBatch) && item.jobId === selectedBatch?.id && item.status === "completed" && Number.isInteger(item.runNumber) && item.runNumber > 0 && (!status || status === "approved" || status === "rejected");
    const canSetPrimary = (item: BatchProductionItem) => canReviewBatchItem(item) && canSetBatchItemPrimary(item);
    const openReview = (item: BatchProductionItem, status: "approved" | "rejected") => {
        if (!canReviewBatchItem(item, status)) {
            message.error("当前结果不可审核");
            return;
        }
        reviewForm.resetFields();
        reviewForm.setFieldsValue({ comment: item.reviewComment || "" });
        setReviewDraft({ item, status });
    };

    const submitReview = async () => {
        const job = selectedBatch;
        if (!job || !reviewDraft || !canReviewBatchItem(reviewDraft.item, reviewDraft.status)) {
            message.error("当前结果不可审核");
            return;
        }
        const value = await reviewForm.validateFields();
        const comment = String(value.comment || "").trim();
        if ((reviewDraft.status === "rejected" && !comment) || comment.length > 1000) {
            message.error(reviewDraft.status === "rejected" && !comment ? "驳回时请填写批注" : "审核批注不能超过 1000 字");
            return;
        }
        const item = reviewDraft.item;
        const reviewStatus = reviewDraft.status;
        setWorking(true);
        try {
            await reviewBatchProductionItem(job.id, item.id, { runNumber: item.runNumber, status: reviewStatus, comment });
            await Promise.all([
                queryClient.invalidateQueries({ queryKey: commerceQueryKeys.jobsRoot(organizationId) }),
                queryClient.invalidateQueries({ queryKey: commerceQueryKeys.job(organizationId, job.id), exact: true }),
                queryClient.invalidateQueries({ queryKey: commerceQueryKeys.jobItemsRoot(organizationId, job.id) }),
            ]);
            message.success(reviewStatus === "approved" ? "审核已通过" : "结果已驳回");
            if (selectedBatchIdRef.current === job.id) setReviewDraft((current) => (current?.item.jobId === job.id && current.item.id === item.id ? null : current));
        } catch (error) {
            message.error(error instanceof Error ? error.message : "审核失败");
        } finally {
            setWorking(false);
        }
    };

    const setPrimary = async (item: BatchProductionItem) => {
        if (!canSetPrimary(item)) {
            message.error("当前结果不可设为主图");
            return false;
        }
        setWorking(true);
        try {
            await setBatchProductionItemPrimary(item.jobId, item.id, item.runNumber);
            await Promise.all([queryClient.invalidateQueries({ queryKey: commerceQueryKeys.job(organizationId, item.jobId), exact: true }), queryClient.invalidateQueries({ queryKey: commerceQueryKeys.jobItemsRoot(organizationId, item.jobId) })]);
            message.success("已设为商品主图");
            return true;
        } catch (error) {
            message.error(error instanceof Error ? error.message : "设置主图失败");
            return false;
        } finally {
            setWorking(false);
        }
    };

    const cancelJob = async (job: BatchProductionJob) => {
        if (!canWrite || !["queued", "running"].includes(job.status)) {
            message.error("当前任务不可取消");
            return;
        }
        await run(() => cancelBatchProductionJob(job.id), "任务已取消");
    };
    const retryJob = async (job: BatchProductionJob) => {
        if (!canWrite || !["failed", "partial_success"].includes(job.status)) {
            message.error("当前任务不可重试");
            return;
        }
        await run(() => retryBatchProductionJob(job.id), "失败项已重新排队");
    };

    const downloadBatchArchive = async (job: BatchProductionJob) => {
        setWorking(true);
        try {
            const result = await downloadBatchProductionArchive(job.id);
            const url = URL.createObjectURL(result.blob);
            const anchor = document.createElement("a");
            anchor.href = url;
            anchor.download = result.filename;
            document.body.appendChild(anchor);
            anchor.click();
            anchor.remove();
            window.setTimeout(() => URL.revokeObjectURL(url), 0);
            message.success("批量结果下载已开始");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "下载批量结果失败");
        } finally {
            setWorking(false);
        }
    };

    const saveBatchItemToAssets = async (item: BatchProductionItem) => {
        if (item.reviewStatus !== "approved" || !item.resultStorageKey) return;
        setWorking(true);
        try {
            const store = useAssetStore.getState();
            const existing = store.assets.find((asset) => asset.metadata?.batchItemId === item.id && asset.metadata?.runNumber === item.runNumber);
            if (!existing) {
                const mediaURL = workspaceFileUrl(item.resultStorageKey);
                store.addAsset({
                    kind: "image",
                    title: `${selectedBatch?.name || "批量结果"} · ${item.skuId || item.productId}`,
                    coverUrl: mediaURL,
                    tags: ["批量生产", ...(item.isPrimary ? ["主图"] : [])],
                    source: "批量生产",
                    note: item.reviewComment,
                    data: { dataUrl: mediaURL, storageKey: item.resultStorageKey, width: 0, height: 0, bytes: 0, mimeType: "image/*" },
                    metadata: { batchJobId: item.jobId, batchItemId: item.id, runNumber: item.runNumber, productId: item.productId, skuId: item.skuId },
                });
            }
            await flushActiveWorkspaceChanges();
            message.success(existing ? "该结果已在素材库" : "结果已保存到素材库");
        } catch (error) {
            message.error(error instanceof Error ? error.message : "保存到素材库失败");
        } finally {
            setWorking(false);
        }
    };

    const saveBatchItemToCanvas = async (item: BatchProductionItem) => {
        if (item.reviewStatus !== "approved" || !item.resultStorageKey) return;
        setWorking(true);
        try {
            const { useCanvasStore } = await import("@/app/(user)/canvas/stores/use-canvas-store");
            const store = useCanvasStore.getState();
            const nodeId = `batch-${item.id}-${item.runNumber}`;
            const existing = store.projects.find((project) => project.nodes.some((node) => node.id === nodeId));
            const projectId =
                existing?.id ||
                store.importProject({
                    title: `${selectedBatch?.name || "批量结果"} · ${item.skuId || item.productId}`,
                    nodes: [
                        {
                            id: nodeId,
                            type: CanvasNodeType.Image,
                            title: item.isPrimary ? "商品主图" : "批量生产结果",
                            position: { x: 80, y: 80 },
                            width: 320,
                            height: 320,
                            metadata: { status: "success", storageKey: item.resultStorageKey, mimeType: "image/*" },
                        },
                    ],
                });
            await flushActiveWorkspaceChanges();
            message.success(existing ? "该结果已在画布中" : "已创建结果画布");
            router.push(`/canvas/${projectId}`);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "添加到画布失败");
        } finally {
            setWorking(false);
        }
    };

    const previewBatchPrompt = async () => {
        const value = await batchForm.validateFields(["presetId", "productIds"]);
        const productId = value.productIds?.[0];
        if (!productId) {
            message.warning("请先选择至少一个商品");
            return;
        }
        setWorking(true);
        try {
            const result = await previewProductionPrompt({
                presetId: value.presetId,
                presetVersion: batchForm.getFieldValue("presetVersion") || 1,
                deliverySpecId: batchForm.getFieldValue("deliverySpecId") || "original",
                brandId: batchForm.getFieldValue("brandId"),
                productId,
                skuId: batchForm.getFieldValue("previewSkuId"),
            });
            setPromptPreview(result.prompt);
        } catch (error) {
            message.error(error instanceof Error ? error.message : "提示词预览失败");
        } finally {
            setWorking(false);
        }
    };

    const openBrand = (item: Partial<Brand>) => {
        brandLogoUploadAbort.current?.abort();
        brandLogoUploadSession.current += 1;
        brandLogoUploadAbort.current = null;
        brandLogoUploadInFlight.current = false;
        setBrandLogoUploading(false);
        brandForm.resetFields();
        setBrandDraft(item);
        brandForm.setFieldsValue({ ...item, colors: item.colors || [], fonts: item.fonts || [], prohibitedTerms: item.prohibitedTerms || [] });
    };
    const openProduct = (item: Partial<Product>) => {
        productForm.resetFields();
        setProductDraft(item);
        productForm.setFieldsValue({ status: "draft", sellingPoints: [], ...item });
    };
    const openSKU = (item: Partial<ProductSKU>) => {
        const next = { productId: selectedProduct?.id, status: "active" as const, imageStorageKeys: [], ...item };
        skuUploadSession.current += 1;
        skuUploadControllers.current.forEach((controller) => controller.abort());
        skuUploadControllers.current.clear();
        skuUploadsInFlightRef.current = 0;
        setSKUUploadsInFlight(0);
        skuForm.resetFields();
        setSkuDraft(next);
        skuForm.setFieldsValue({ ...next, attributesText: JSON.stringify(next.attributes || {}, null, 2) });
    };
    const closeSKU = () => {
        if (skuUploadsInFlightRef.current > 0) {
            skuUploadSession.current += 1;
            skuUploadControllers.current.forEach((controller) => controller.abort());
            skuUploadControllers.current.clear();
            skuUploadsInFlightRef.current = 0;
            setSKUUploadsInFlight(0);
        }
        setSkuDraft(null);
    };
    const submitSKU = async () => {
        if (skuUploadsInFlightRef.current > 0) {
            message.warning("请等待参考图上传完成");
            return;
        }
        const value = await skuForm.validateFields();
        const { attributesText, ...fields } = value;
        let attributes: Record<string, string> = {};
        try {
            attributes = JSON.parse(attributesText || "{}") as Record<string, string>;
        } catch {
            message.error("规格属性必须是有效 JSON");
            return;
        }
        if (await run(() => saveProductSKU({ ...skuDraft, ...fields, productId: selectedProduct?.id, attributes }), "SKU 已保存")) setSkuDraft(null);
    };
    const closeBrand = () => {
        brandLogoUploadSession.current += 1;
        brandLogoUploadAbort.current?.abort();
        brandLogoUploadAbort.current = null;
        brandLogoUploadInFlight.current = false;
        setBrandLogoUploading(false);
        setBrandDraft(null);
    };
    const submitBrand = async () => {
        if (brandLogoUploadInFlight.current) {
            message.warning("请等待 Logo 上传完成");
            return;
        }
        const value = await brandForm.validateFields();
        if (await run(() => saveBrand({ ...brandDraft, ...value }), "品牌规范已保存")) closeBrand();
    };

    if (!workspace && workspaceQuery.isLoading) return <main className="grid h-full place-items-center bg-background text-sm text-muted-foreground">正在建立企业工作区...</main>;

    const organizationPanel = (
        <div className="flex flex-col gap-5">
            {(invitationsQuery.data || []).length ? (
                <Card className="border-primary/30 bg-primary/5 dark:border-primary/40 dark:bg-primary/10">
                    <div className="mb-3 text-sm font-medium">待接受的企业邀请</div>
                    <div className="flex flex-wrap gap-2">
                        {invitationsQuery.data?.map((item) => (
                            <Button
                                key={item.id}
                                onClick={() =>
                                    void run(
                                        () =>
                                            changeOrganizationContext(async () => {
                                                await acceptOrganizationInvitation(item.id);
                                                return item.organizationId;
                                            }),
                                        "已加入企业",
                                        false,
                                    )
                                }
                            >
                                {item.organizationName || "企业邀请"} · {roleLabels[item.role]}
                            </Button>
                        ))}
                    </div>
                </Card>
            ) : null}
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
                {[
                    { label: "品牌", value: workspace?.stats.brands || 0, icon: Palette },
                    { label: "SPU", value: workspace?.stats.products || 0, icon: Boxes },
                    { label: "SKU", value: workspace?.stats.skus || 0, icon: PackagePlus },
                    { label: "批量任务", value: workspace?.stats.batchJobs || 0, icon: ClipboardList },
                    { label: "待生产", value: workspace?.stats.pendingItems || 0, icon: RefreshCw },
                ].map((item) => {
                    const Icon = item.icon;
                    return (
                        <Card key={item.label}>
                            <Statistic
                                title={
                                    <span className="inline-flex items-center gap-2">
                                        <Icon className="size-4" />
                                        {item.label}
                                    </span>
                                }
                                value={item.value}
                            />
                        </Card>
                    );
                })}
            </div>
            <Card
                title={
                    <span className="inline-flex items-center gap-2">
                        <WalletCards className="size-4" />
                        企业算力
                    </span>
                }
                extra={
                    canManage ? (
                        <Button
                            size="small"
                            icon={<Settings2 className="size-4" />}
                            onClick={() => {
                                creditSettingsForm.setFieldsValue({ mode: creditSummary?.mode || "personal", monthlyBudget: creditSummary?.monthlyBudget || 0, alertThreshold: creditSummary?.alertThreshold || 80 });
                                setCreditSettingsOpen(true);
                            }}
                        >
                            额度设置
                        </Button>
                    ) : null
                }
            >
                <div className="grid gap-5 md:grid-cols-3">
                    <div>
                        <div className="text-xs text-muted-foreground">当前扣费模式</div>
                        <div className="mt-2">
                            <Tag color={creditSummary?.mode === "shared" ? "blue" : "default"}>{creditSummary?.mode === "shared" ? "企业共享额度" : "成员个人额度"}</Tag>
                        </div>
                    </div>
                    <div>
                        <div className="text-xs text-muted-foreground">企业共享余额</div>
                        <div className="mt-1 text-2xl font-semibold tabular-nums">
                            {(creditSummary?.balance || 0).toLocaleString()}
                            <span className="ml-1 text-xs font-normal text-muted-foreground">点</span>
                        </div>
                    </div>
                    <div>
                        <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
                            <span>本月已用</span>
                            <span className="tabular-nums">
                                {(creditSummary?.monthlyUsed || 0).toLocaleString()} / {creditSummary?.monthlyBudget ? creditSummary.monthlyBudget.toLocaleString() : "不限"}
                            </span>
                        </div>
                        <Progress className="mt-2" percent={budgetPercent} showInfo={false} status={creditSummary?.warning ? "exception" : "normal"} />
                    </div>
                </div>
                {creditSummary?.warning ? <Alert className="mt-4" type="warning" showIcon message={`本月算力已达到预算预警线（${creditSummary.alertThreshold}%）`} /> : null}
                <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
                    <span className="text-sm text-muted-foreground">
                        我的个人算力：<span className="font-medium tabular-nums text-foreground">{(creditSummary?.personalBalance || 0).toLocaleString()} 点</span>
                    </span>
                    <Button
                        disabled={!creditSummary?.personalBalance}
                        onClick={() => {
                            creditTransferForm.resetFields();
                            setCreditTransferOpen(true);
                        }}
                    >
                        从个人算力转入
                    </Button>
                </div>
            </Card>
            <div className="grid gap-5 xl:grid-cols-[1fr_1.6fr]">
                <Card
                    title="企业信息"
                    extra={
                        canManage ? (
                            <Button
                                size="small"
                                icon={<Settings2 className="size-4" />}
                                onClick={() => {
                                    organizationForm.setFieldValue("name", workspace?.organization.name);
                                    setOrganizationMode("edit");
                                }}
                            >
                                设置
                            </Button>
                        ) : null
                    }
                >
                    <Descriptions
                        column={1}
                        size="small"
                        items={[
                            { key: "name", label: "当前企业", children: workspace?.organization.name },
                            { key: "slug", label: "企业标识", children: workspace?.organization.slug },
                            { key: "role", label: "我的角色", children: <Tag>{roleLabels[workspace?.membership.role || ""]}</Tag> },
                        ]}
                    />
                    <div className="mt-5 border-t border-border pt-4">
                        <div className="mb-2 text-xs text-muted-foreground">切换企业</div>
                        <Select
                            className="w-full"
                            value={workspace?.organization.id}
                            disabled={working}
                            options={workspace?.organizations.map((item) => ({ value: item.id, label: `${item.name} · ${roleLabels[item.role]}` }))}
                            onChange={(value) =>
                                void run(
                                    () =>
                                        changeOrganizationContext(async () => {
                                            await switchOrganization(value);
                                            return value;
                                        }),
                                    "已切换企业",
                                    false,
                                )
                            }
                        />
                        <Button
                            className="mt-3"
                            block
                            icon={<Plus className="size-4" />}
                            onClick={() => {
                                organizationForm.resetFields();
                                setOrganizationMode("create");
                            }}
                        >
                            创建新企业
                        </Button>
                    </div>
                </Card>
                <Card
                    title={
                        <span className="inline-flex items-center gap-2">
                            <Users className="size-4" />
                            企业成员
                        </span>
                    }
                    extra={
                        <div className="flex gap-2">
                            <Input.Search allowClear placeholder="搜索成员" className="w-48" onSearch={(keyword) => setMemberListQuery((value) => ({ ...value, page: 1, keyword }))} />
                            {canManage ? (
                                <Button
                                    type="primary"
                                    size="small"
                                    icon={<UserPlus className="size-4" />}
                                    onClick={() => {
                                        inviteForm.setFieldsValue({ role: "member" });
                                        setInviteOpen(true);
                                    }}
                                >
                                    邀请成员
                                </Button>
                            ) : null}
                        </div>
                    }
                >
                    <Table<OrganizationMember>
                        rowKey="id"
                        size="small"
                        loading={membersQuery.isFetching}
                        dataSource={membersQuery.data?.items || []}
                        pagination={{
                            current: memberListQuery.page,
                            pageSize: memberListQuery.pageSize,
                            total: membersQuery.data?.total || 0,
                            showSizeChanger: true,
                            onChange: (page, pageSize) => setMemberListQuery((value) => ({ ...value, page, pageSize })),
                        }}
                        columns={[
                            {
                                title: "成员",
                                render: (_, item) => (
                                    <div className="flex items-center gap-2">
                                        <Avatar src={item.avatarUrl}>{(item.displayName || item.username).slice(0, 1)}</Avatar>
                                        <div>
                                            <div className="text-sm font-medium">{item.displayName || item.username}</div>
                                            <div className="text-xs text-muted-foreground">{item.email}</div>
                                        </div>
                                    </div>
                                ),
                            },
                            {
                                title: "角色",
                                width: 130,
                                render: (_, item) =>
                                    item.role === "owner" || !canManage ? (
                                        <Tag>{roleLabels[item.role]}</Tag>
                                    ) : (
                                        <Select size="small" value={item.role} options={roleOptions} onChange={(role) => void run(() => updateOrganizationMember(item.id, role, item.version), "成员角色已更新")} />
                                    ),
                            },
                            {
                                title: "",
                                width: 150,
                                render: (_, item) =>
                                    canManage && item.role !== "owner" ? (
                                        <div className="flex justify-end gap-1">
                                            {workspace?.membership.role === "owner" ? (
                                                <Popconfirm title="将企业所有权转移给该成员？" onConfirm={() => void run(() => transferOrganizationOwnership(item.id, item.version), "企业所有权已转移")}>
                                                    <Button size="small">设为所有者</Button>
                                                </Popconfirm>
                                            ) : null}
                                            <Popconfirm title="移除该成员？" onConfirm={() => void run(() => removeOrganizationMember(item.id, item.version), "成员已移除")}>
                                                <Button danger type="text" icon={<Trash2 className="size-4" />} />
                                            </Popconfirm>
                                        </div>
                                    ) : null,
                            },
                        ]}
                    />
                    {canManage && organizationInvitationsQuery.data?.some((item) => item.status === "pending") ? (
                        <div className="mt-4 border-t border-border pt-4">
                            <div className="mb-2 text-xs text-muted-foreground">待接受邀请</div>
                            <div className="flex flex-wrap gap-2">
                                {organizationInvitationsQuery.data
                                    .filter((item) => item.status === "pending")
                                    .map((item) => (
                                        <Tag
                                            key={item.id}
                                            closable
                                            onClose={(event) => {
                                                event.preventDefault();
                                                void run(() => revokeOrganizationInvitation(item.id), "邀请已撤销");
                                            }}
                                        >
                                            {item.email} · {roleLabels[item.role]}
                                        </Tag>
                                    ))}
                            </div>
                        </div>
                    ) : null}
                </Card>
            </div>
        </div>
    );

    const brandPanel = (
        <div>
            <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
                <div>
                    <h2 className="text-2xl font-semibold">品牌规范中心</h2>
                    <p className="mt-1 text-sm text-muted-foreground">统一 Logo、颜色、字体、语气与禁止规则。</p>
                </div>
                <div className="flex gap-2">
                    <Input.Search allowClear placeholder="搜索品牌" className="w-56" onSearch={(keyword) => setBrandListQuery((value) => ({ ...value, page: 1, keyword }))} />
                    {canWrite ? (
                        <Button type="primary" icon={<Plus className="size-4" />} onClick={() => openBrand({})}>
                            新增品牌
                        </Button>
                    ) : null}
                </div>
            </div>
            {brands.length ? (
                <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                    {brands.map((item) => (
                        <Card
                            key={item.id}
                            className="overflow-hidden"
                            actions={
                                canWrite
                                    ? [
                                          <button key="edit" onClick={() => openBrand(item)}>
                                              编辑规范
                                          </button>,
                                          <Popconfirm key="delete" title="删除该品牌？" onConfirm={() => void run(() => deleteBrand(item.id, item.version), "品牌已删除")}>
                                              <button className="text-red-500">删除</button>
                                          </Popconfirm>,
                                      ]
                                    : undefined
                            }
                        >
                            <div className="flex items-start gap-3">
                                {item.logoStorageKey ? (
                                    <Avatar shape="square" size={48} src={workspaceFileUrl(item.logoStorageKey)} />
                                ) : (
                                    <div className="grid size-12 place-items-center bg-neutral-950 text-lg font-semibold text-white">{item.name.slice(0, 1)}</div>
                                )}
                                <div>
                                    <h3 className="text-lg font-semibold">{item.name}</h3>
                                    <p className="mt-1 line-clamp-2 text-sm text-muted-foreground">{item.tone || "尚未填写品牌语气"}</p>
                                </div>
                            </div>
                            <div className="mt-5 flex gap-2">
                                {item.colors?.map((color) => (
                                    <span key={color} className="size-7 border border-black/10" style={{ background: color }} title={color} />
                                ))}
                            </div>
                            <p className="mt-4 line-clamp-3 min-h-15 text-sm leading-5 text-muted-foreground">{item.guidelines || "尚未填写视觉规范"}</p>
                        </Card>
                    ))}
                </div>
            ) : (
                <Empty description="还没有品牌规范" />
            )}
            <div className="mt-5 flex justify-end">
                <Pagination current={brandListQuery.page} pageSize={brandListQuery.pageSize} total={brandsQuery.data?.total || 0} showSizeChanger onChange={(page, pageSize) => setBrandListQuery((value) => ({ ...value, page, pageSize }))} />
            </div>
        </div>
    );

    const productPanel = (
        <div>
            <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
                <div>
                    <h2 className="text-2xl font-semibold">商品 / SKU 中心</h2>
                    <p className="mt-1 text-sm text-muted-foreground">以 SPU 组织卖点，以 SKU 维护规格与参考图。</p>
                </div>
                <div className="flex gap-2">
                    <Input.Search allowClear placeholder="搜索名称、SPU 或类目" className="w-64" onSearch={(keyword) => setProductListQuery((value) => ({ ...value, page: 1, keyword }))} />
                    {canWrite ? (
                        <Button type="primary" icon={<Plus className="size-4" />} onClick={() => openProduct({})}>
                            新增商品
                        </Button>
                    ) : null}
                </div>
            </div>
            <Card>
                <Table<Product>
                    rowKey="id"
                    loading={productsQuery.isFetching}
                    dataSource={products}
                    pagination={{
                        current: productListQuery.page,
                        pageSize: productListQuery.pageSize,
                        total: productsQuery.data?.total || 0,
                        showSizeChanger: true,
                        onChange: (page, pageSize) => setProductListQuery((value) => ({ ...value, page, pageSize })),
                    }}
                    columns={[
                        { title: "SPU", dataIndex: "code", width: 150, render: (value) => <span className="font-mono text-xs">{value}</span> },
                        {
                            title: "商品",
                            render: (_, item) => (
                                <div>
                                    <div className="font-medium">{item.name}</div>
                                    <div className="mt-1 text-xs text-muted-foreground">{item.category || "未分类"}</div>
                                </div>
                            ),
                        },
                        { title: "品牌", dataIndex: "brandName", width: 140, render: (value) => value || "—" },
                        { title: "SKU", dataIndex: "skuCount", width: 80 },
                        { title: "状态", width: 90, render: (_, item) => <Tag color={statusColors[item.status]}>{statusLabels[item.status]}</Tag> },
                        {
                            title: "操作",
                            width: 210,
                            render: (_, item) => (
                                <div className="flex gap-1">
                                    <Button
                                        size="small"
                                        onClick={() => {
                                            setSKUListQuery(initialListQuery);
                                            setSelectedProduct(item);
                                        }}
                                    >
                                        管理 SKU
                                    </Button>
                                    {canWrite ? (
                                        <>
                                            <Button size="small" onClick={() => openProduct(item)}>
                                                编辑
                                            </Button>
                                            <Popconfirm title="删除商品及其 SKU？" onConfirm={() => void run(() => deleteProduct(item.id, item.version), "商品已删除")}>
                                                <Button danger size="small">
                                                    删除
                                                </Button>
                                            </Popconfirm>
                                        </>
                                    ) : null}
                                </div>
                            ),
                        },
                    ]}
                />
            </Card>
        </div>
    );

    const batchPanel = (
        <div>
            <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
                <div>
                    <h2 className="text-2xl font-semibold">批量生产任务</h2>
                    <p className="mt-1 text-sm text-muted-foreground">按商品与 SKU 展开服务端持久化任务项，支持审核、渠道交付和进度追踪。</p>
                </div>
                <div className="flex gap-2">
                    <Input.Search allowClear placeholder="搜索任务" className="w-56" onSearch={(keyword) => setJobListQuery((value) => ({ ...value, page: 1, keyword }))} />
                    {canWrite ? (
                        <Button
                            type="primary"
                            icon={<Plus className="size-4" />}
                            onClick={() => {
                                batchRequestId.current = crypto.randomUUID();
                                batchForm.resetFields();
                                batchForm.setFieldsValue({ deliverySpecId: "original" });
                                setBatchOpen(true);
                            }}
                        >
                            创建任务
                        </Button>
                    ) : null}
                </div>
            </div>
            <Card>
                <Table<BatchProductionJob>
                    rowKey="id"
                    loading={jobsQuery.isFetching}
                    dataSource={jobs}
                    pagination={{ current: jobListQuery.page, pageSize: jobListQuery.pageSize, total: jobsQuery.data?.total || 0, showSizeChanger: true, onChange: (page, pageSize) => setJobListQuery((value) => ({ ...value, page, pageSize })) }}
                    columns={[
                        {
                            title: "任务",
                            render: (_, item) => (
                                <div>
                                    <div className="font-medium">{item.name}</div>
                                    <div className="mt-1 text-xs text-muted-foreground">
                                        {productionTemplateOptions.find((template) => template.id === item.presetId)?.title || item.presetId} · v{item.presetVersion || 1} · {item.deliverySpec?.platform || "通用"}
                                    </div>
                                </div>
                            ),
                        },
                        { title: "状态", width: 100, render: (_, item) => <Tag color={statusColors[item.status]}>{statusLabels[item.status]}</Tag> },
                        {
                            title: "进度",
                            width: 220,
                            render: (_, item) => (
                                <Progress
                                    size="small"
                                    percent={item.totalItems ? Math.round(((item.completedItems + item.failedItems) / item.totalItems) * 100) : 0}
                                    format={() => `${item.completedItems + item.failedItems}/${item.totalItems}${item.failedItems ? ` · ${item.failedItems} 失败` : ""}`}
                                />
                            ),
                        },
                        {
                            title: "操作",
                            width: 290,
                            render: (_, item) => (
                                <div className="flex gap-1">
                                    <Button
                                        size="small"
                                        onClick={() => {
                                            setItemListQuery(initialListQuery);
                                            setComparisonItems([]);
                                            setComparisonOpen(false);
                                            setSelectedBatch(item);
                                        }}
                                    >
                                        任务项
                                    </Button>
                                    {item.completedItems > 0 ? (
                                        <Button size="small" icon={<Download className="size-3.5" />} onClick={() => void downloadBatchArchive(item)}>
                                            下载结果
                                        </Button>
                                    ) : null}
                                    {canWrite && ["queued", "running"].includes(item.status) ? (
                                        <Button size="small" onClick={() => void cancelJob(item)}>
                                            取消
                                        </Button>
                                    ) : null}
                                    {canWrite && ["failed", "partial_success"].includes(item.status) ? (
                                        <Button size="small" onClick={() => void retryJob(item)}>
                                            重试
                                        </Button>
                                    ) : null}
                                </div>
                            ),
                        },
                    ]}
                />
            </Card>
        </div>
    );

    const templatePanel = (
        <div>
            <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
                <div>
                    <h2 className="text-2xl font-semibold">企业生产模板</h2>
                    <p className="mt-1 text-sm text-muted-foreground">版本化维护企业专属生产指令，历史任务始终使用创建时版本。</p>
                </div>
                {canManage ? (
                    <Button type="primary" icon={<Plus className="size-4" />} onClick={() => router.push("/commerce/templates")}>
                        管理模板
                    </Button>
                ) : null}
            </div>
            <Card>
                <Table<ProductionTemplate>
                    rowKey="id"
                    loading={templatesQuery.isFetching}
                    dataSource={productionTemplates}
                    pagination={false}
                    columns={[
                        {
                            title: "模板",
                            render: (_, item) => (
                                <div>
                                    <div className="font-medium">{item.name}</div>
                                    <div className="mt-1 text-xs text-muted-foreground">{item.description || "暂无说明"}</div>
                                </div>
                            ),
                        },
                        { title: "状态", width: 100, render: (_, item) => <Tag color={item.status === "active" ? "green" : "default"}>{item.status === "active" ? "启用" : "停用"}</Tag> },
                        { title: "当前版本", width: 100, render: (_, item) => `v${item.currentVersion}` },
                        { title: "更新时间", dataIndex: "updatedAt", width: 190 },
                        {
                            title: "操作",
                            width: 150,
                            render: (_, item) => (
                                <div className="flex gap-1">
                                    <Button size="small" onClick={() => setSelectedTemplate(item)}>
                                        版本
                                    </Button>
                                    {canManage ? (
                                        <Button size="small" onClick={() => router.push("/commerce/templates")}>
                                            编辑
                                        </Button>
                                    ) : null}
                                </div>
                            ),
                        },
                    ]}
                />
            </Card>
        </div>
    );

    const auditPanel = (
        <Card title="企业审计日志" extra={<Input.Search allowClear placeholder="搜索动作或资源" className="w-56" onSearch={(keyword) => setAuditListQuery((value) => ({ ...value, page: 1, keyword }))} />}>
            <Table
                rowKey="id"
                loading={auditQuery.isFetching}
                dataSource={auditQuery.data?.items || []}
                pagination={{ current: auditListQuery.page, pageSize: auditListQuery.pageSize, total: auditQuery.data?.total || 0, showSizeChanger: true, onChange: (page, pageSize) => setAuditListQuery((value) => ({ ...value, page, pageSize })) }}
                columns={[
                    { title: "时间", dataIndex: "createdAt", width: 190 },
                    { title: "动作", dataIndex: "action", width: 180 },
                    { title: "资源", render: (_, item) => `${item.resourceType} / ${item.resourceId}` },
                    { title: "操作者", dataIndex: "userId", width: 230 },
                ]}
            />
        </Card>
    );

    return (
        <div className="bg-background">
            <div className="mx-auto max-w-[1440px] px-4 py-8 sm:px-6 lg:px-8 lg:py-12">
                <header className="hero-atmosphere mb-9 flex min-h-64 flex-wrap items-end justify-between gap-6 rounded-xl border border-border bg-card p-7 sm:p-10 dark:rounded-none dark:border-b dark:border-border dark:bg-transparent dark:p-0 dark:py-12 dark:sm:py-14">
                    <div>
                        <div className="mb-3 inline-flex items-center gap-2 text-sm font-medium text-primary">
                            <Building2 className="size-4" /> 企业电商工作区
                        </div>
                        <h1 className="text-5xl font-semibold tracking-[-.045em] sm:text-6xl">{workspace?.organization.name || "企业中心"}</h1>
                        <p className="mt-4 max-w-2xl text-base leading-7 text-muted-foreground">品牌资产、商品主数据与视觉生产任务，在一个工作区持续协作。</p>
                    </div>
                    <Button icon={<RefreshCw className="size-4" />} onClick={() => void invalidate()}>
                        刷新数据
                    </Button>
                </header>
                <Tabs
                    size="large"
                    activeKey={activeTab}
                    onChange={setActiveTab}
                    items={[
                        {
                            key: "organization",
                            label: (
                                <span className="inline-flex items-center gap-2">
                                    <Users className="size-4" />
                                    企业与成员
                                </span>
                            ),
                            children: organizationPanel,
                        },
                        {
                            key: "brands",
                            label: (
                                <span className="inline-flex items-center gap-2">
                                    <Palette className="size-4" />
                                    品牌中心
                                </span>
                            ),
                            children: brandPanel,
                        },
                        {
                            key: "products",
                            label: (
                                <span className="inline-flex items-center gap-2">
                                    <Boxes className="size-4" />
                                    商品 / SKU
                                </span>
                            ),
                            children: productPanel,
                        },
                        {
                            key: "templates",
                            label: (
                                <span className="inline-flex items-center gap-2">
                                    <FileStack className="size-4" />
                                    生产模板
                                </span>
                            ),
                            children: templatePanel,
                        },
                        {
                            key: "batch",
                            label: (
                                <span className="inline-flex items-center gap-2">
                                    <ClipboardList className="size-4" />
                                    批量生产
                                </span>
                            ),
                            children: batchPanel,
                        },
                        ...(canManage
                            ? [
                                  {
                                      key: "audit",
                                      label: (
                                          <span className="inline-flex items-center gap-2">
                                              <History className="size-4" />
                                              审计日志
                                          </span>
                                      ),
                                      children: auditPanel,
                                  },
                              ]
                            : []),
                    ]}
                />
            </div>

            <Modal
                title={organizationMode === "edit" ? "企业设置" : "创建企业"}
                open={Boolean(organizationMode)}
                confirmLoading={working}
                onCancel={() => setOrganizationMode(null)}
                onOk={async () => {
                    const value = await organizationForm.validateFields();
                    const editing = organizationMode === "edit";
                    const ok = editing
                        ? await run(() => updateOrganization(value.name, workspace?.organization.version || 0), "企业信息已更新")
                        : await run(() => changeOrganizationContext(async () => (await createOrganization(value.name)).id), "企业已创建", false);
                    if (ok) setOrganizationMode(null);
                }}
            >
                <Form form={organizationForm} layout="vertical">
                    <Form.Item name="name" label="企业名称" rules={[{ required: true, max: 200 }]}>
                        <Input />
                    </Form.Item>
                </Form>
            </Modal>
            <Modal
                title="企业额度设置"
                open={creditSettingsOpen}
                confirmLoading={working}
                onCancel={() => setCreditSettingsOpen(false)}
                onOk={async () => {
                    const value = await creditSettingsForm.validateFields();
                    const ok = await run(async () => {
                        await updateOrganizationCreditSettings({ ...value, version: workspace?.organization.version || 0 });
                        await refreshUser();
                    }, "企业额度设置已更新");
                    if (ok) setCreditSettingsOpen(false);
                }}
            >
                <Form form={creditSettingsForm} layout="vertical" requiredMark={false}>
                    <Form.Item name="mode" label="成员生成任务扣费方式" rules={[{ required: true }]}>
                        <Select
                            options={[
                                { value: "personal", label: "成员个人额度" },
                                { value: "shared", label: "企业共享额度" },
                            ]}
                        />
                    </Form.Item>
                    <Form.Item name="monthlyBudget" label="月度预算" extra="设为 0 表示不限制。" rules={[{ required: true }]}>
                        <InputNumber className="w-full" min={0} max={1000000000} precision={0} addonAfter="点" />
                    </Form.Item>
                    <Form.Item name="alertThreshold" label="预算预警线" rules={[{ required: true }]}>
                        <InputNumber className="w-full" min={1} max={100} precision={0} addonAfter="%" />
                    </Form.Item>
                </Form>
            </Modal>
            <Modal
                title="转入企业共享额度"
                open={creditTransferOpen}
                confirmLoading={working}
                onCancel={() => setCreditTransferOpen(false)}
                onOk={async () => {
                    const value = await creditTransferForm.validateFields();
                    const ok = await run(async () => {
                        await transferOrganizationCredits(value.amount);
                        await refreshUser();
                    }, "额度已转入企业共享池");
                    if (ok) setCreditTransferOpen(false);
                }}
            >
                <Form form={creditTransferForm} layout="vertical" requiredMark={false}>
                    <Form.Item name="amount" label="转入额度" extra={`当前个人算力 ${(creditSummary?.personalBalance || 0).toLocaleString()} 点`} rules={[{ required: true }]}>
                        <InputNumber className="w-full" min={1} max={Math.max(1, creditSummary?.personalBalance || 0)} precision={0} addonAfter="点" />
                    </Form.Item>
                </Form>
            </Modal>
            <Modal
                title="邀请企业成员"
                open={inviteOpen}
                confirmLoading={working}
                onCancel={() => setInviteOpen(false)}
                onOk={async () => {
                    const value = await inviteForm.validateFields();
                    if (await run(() => inviteOrganizationMember(value.email, value.role), "邀请已创建")) setInviteOpen(false);
                }}
            >
                <Form form={inviteForm} layout="vertical">
                    <Form.Item name="email" label="成员邮箱" rules={[{ required: true, type: "email" }]}>
                        <Input />
                    </Form.Item>
                    <Form.Item name="role" label="角色" rules={[{ required: true }]}>
                        <Select options={roleOptions} />
                    </Form.Item>
                </Form>
            </Modal>
            <Modal title={brandDraft?.id ? "编辑品牌规范" : "新增品牌"} open={Boolean(brandDraft)} width={760} confirmLoading={working || brandLogoUploading} onCancel={closeBrand} onOk={submitBrand}>
                <Form form={brandForm} layout="vertical">
                    <div className="grid gap-x-4 sm:grid-cols-2">
                        <Form.Item name="name" label="品牌名称" rules={[{ required: true }]}>
                            <Input />
                        </Form.Item>
                        <Form.Item label="品牌 Logo">
                            <div className="flex items-center gap-3">
                                {brandLogoStorageKey ? <Avatar shape="square" size={48} src={workspaceFileUrl(brandLogoStorageKey)} /> : null}
                                <Upload
                                    accept="image/*"
                                    maxCount={1}
                                    showUploadList={false}
                                    beforeUpload={(file) => {
                                        if (!file.type.startsWith("image/")) {
                                            message.error("只能上传图片文件");
                                            return Upload.LIST_IGNORE;
                                        }
                                        if (brandLogoUploadInFlight.current) {
                                            message.warning("请等待当前 Logo 上传完成");
                                            return Upload.LIST_IGNORE;
                                        }
                                        brandUploadFileSessions.current.set(file, brandLogoUploadSession.current);
                                        brandLogoUploadInFlight.current = true;
                                        setBrandLogoUploading(true);
                                        return true;
                                    }}
                                    customRequest={({ file, onSuccess, onError }) => {
                                        const session = brandUploadFileSessions.current.get(file as object);
                                        if (session !== brandLogoUploadSession.current) return { abort: () => undefined };
                                        const controller = new AbortController();
                                        brandLogoUploadAbort.current = controller;
                                        void uploadWorkspaceFile(token, `image:${crypto.randomUUID()}`, file as File, controller.signal)
                                            .then((saved) => {
                                                if (session !== brandLogoUploadSession.current) return;
                                                brandForm.setFieldValue("logoStorageKey", saved.storageKey);
                                                onSuccess?.(saved);
                                            })
                                            .catch((error) => {
                                                if (session !== brandLogoUploadSession.current || controller.signal.aborted) return;
                                                message.error(error instanceof Error ? error.message : "Logo 上传失败");
                                                onError?.(error as Error);
                                            })
                                            .finally(() => {
                                                if (session === brandLogoUploadSession.current) {
                                                    brandLogoUploadAbort.current = null;
                                                    brandLogoUploadInFlight.current = false;
                                                    setBrandLogoUploading(false);
                                                }
                                            });
                                        return { abort: () => controller.abort() };
                                    }}
                                >
                                    <Button loading={brandLogoUploading} disabled={brandLogoUploading}>
                                        上传 Logo
                                    </Button>
                                </Upload>
                                {brandLogoStorageKey ? (
                                    <Button type="text" danger disabled={brandLogoUploading} onClick={() => brandForm.setFieldValue("logoStorageKey", "")}>
                                        移除
                                    </Button>
                                ) : null}
                            </div>
                        </Form.Item>
                        <Form.Item name="logoStorageKey" hidden>
                            <Input />
                        </Form.Item>
                        <Form.Item name="colors" label="品牌色">
                            <Select mode="tags" placeholder="#111111" />
                        </Form.Item>
                        <Form.Item name="fonts" label="品牌字体">
                            <Select mode="tags" />
                        </Form.Item>
                    </div>
                    <Form.Item name="tone" label="品牌语气">
                        <Input.TextArea rows={2} />
                    </Form.Item>
                    <Form.Item name="guidelines" label="视觉规范">
                        <Input.TextArea rows={4} />
                    </Form.Item>
                    <Form.Item name="prohibitedTerms" label="禁用词">
                        <Select mode="tags" />
                    </Form.Item>
                </Form>
            </Modal>
            <Modal
                title={productDraft?.id ? "编辑商品" : "新增商品"}
                open={Boolean(productDraft)}
                width={760}
                confirmLoading={working}
                onCancel={() => setProductDraft(null)}
                onOk={async () => {
                    const value = await productForm.validateFields();
                    if (await run(() => saveProduct({ ...productDraft, ...value }), "商品已保存")) setProductDraft(null);
                }}
            >
                <Form form={productForm} layout="vertical">
                    <div className="grid gap-x-4 sm:grid-cols-2">
                        <Form.Item name="code" label="SPU 编码" rules={[{ required: true }]}>
                            <Input />
                        </Form.Item>
                        <Form.Item name="name" label="商品名称" rules={[{ required: true }]}>
                            <Input />
                        </Form.Item>
                        <Form.Item name="brandId" label="所属品牌">
                            <Select allowClear showSearch filterOption={false} onSearch={setBrandOptionKeyword} loading={brandOptionsQuery.isFetching} options={brandOptions.map((item) => ({ value: item.id, label: item.name }))} />
                        </Form.Item>
                        <Form.Item name="category" label="商品类目">
                            <Input />
                        </Form.Item>
                        <Form.Item name="status" label="状态">
                            <Select
                                options={[
                                    { value: "draft", label: "草稿" },
                                    { value: "active", label: "在售" },
                                    { value: "paused", label: "暂停" },
                                ]}
                            />
                        </Form.Item>
                        <Form.Item name="targetAudience" label="目标人群">
                            <Input />
                        </Form.Item>
                    </div>
                    <Form.Item name="sellingPoints" label="核心卖点">
                        <Select mode="tags" />
                    </Form.Item>
                    <Form.Item name="description" label="商品描述">
                        <Input.TextArea rows={4} />
                    </Form.Item>
                </Form>
            </Modal>
            <Drawer
                title={`${selectedProduct?.name || "商品"} · SKU`}
                width={820}
                open={Boolean(selectedProduct)}
                onClose={() => setSelectedProduct(null)}
                extra={
                    canWrite ? (
                        <Button type="primary" icon={<Plus className="size-4" />} onClick={() => openSKU({})}>
                            新增 SKU
                        </Button>
                    ) : null
                }
            >
                <Input.Search allowClear placeholder="搜索 SKU 编码或名称" className="mb-4 w-64" onSearch={(keyword) => setSKUListQuery((value) => ({ ...value, page: 1, keyword }))} />
                <Table<ProductSKU>
                    rowKey="id"
                    loading={skusQuery.isFetching}
                    dataSource={skusQuery.data?.items || []}
                    pagination={{ current: skuListQuery.page, pageSize: skuListQuery.pageSize, total: skusQuery.data?.total || 0, showSizeChanger: true, onChange: (page, pageSize) => setSKUListQuery((value) => ({ ...value, page, pageSize })) }}
                    columns={[
                        { title: "SKU 编码", dataIndex: "code", width: 150 },
                        { title: "名称", dataIndex: "name" },
                        {
                            title: "规格",
                            render: (_, item) =>
                                Object.entries(item.attributes || {}).map(([key, value]) => (
                                    <Tag key={key}>
                                        {key}: {value}
                                    </Tag>
                                )),
                        },
                        { title: "状态", width: 80, render: (_, item) => <Tag color={statusColors[item.status]}>{statusLabels[item.status]}</Tag> },
                        {
                            title: "",
                            width: 120,
                            render: (_, item) =>
                                canWrite ? (
                                    <div className="flex gap-1">
                                        <Button size="small" onClick={() => openSKU(item)}>
                                            编辑
                                        </Button>
                                        <Popconfirm title="删除 SKU？" onConfirm={() => void run(() => deleteProductSKU(item.id, item.version), "SKU 已删除")}>
                                            <Button danger size="small">
                                                删除
                                            </Button>
                                        </Popconfirm>
                                    </div>
                                ) : null,
                        },
                    ]}
                />
            </Drawer>
            <Modal title={skuDraft?.id ? "编辑 SKU" : "新增 SKU"} open={Boolean(skuDraft)} width={680} confirmLoading={working || skuUploadsInFlight > 0} onCancel={closeSKU} onOk={submitSKU}>
                <Form form={skuForm} layout="vertical">
                    <div className="grid gap-x-4 sm:grid-cols-2">
                        <Form.Item name="code" label="SKU 编码" rules={[{ required: true }]}>
                            <Input />
                        </Form.Item>
                        <Form.Item name="name" label="SKU 名称" rules={[{ required: true }]}>
                            <Input />
                        </Form.Item>
                        <Form.Item name="status" label="状态">
                            <Select
                                options={[
                                    { value: "active", label: "在售" },
                                    { value: "paused", label: "暂停" },
                                ]}
                            />
                        </Form.Item>
                    </div>
                    <Form.Item name="attributesText" label="规格属性 JSON">
                        <Input.TextArea rows={4} placeholder={'{"颜色":"黑色","尺寸":"M"}'} />
                    </Form.Item>
                    <Form.Item label="商品参考图">
                        <div className="mb-3 flex flex-wrap gap-2">
                            {skuImageStorageKeys.map((storageKey: string) => (
                                <div key={storageKey} className="relative">
                                    <Avatar shape="square" size={56} src={workspaceFileUrl(storageKey)} />
                                    <Button
                                        className="absolute -right-2 -top-2"
                                        size="small"
                                        danger
                                        shape="circle"
                                        onClick={() =>
                                            skuForm.setFieldValue(
                                                "imageStorageKeys",
                                                skuImageStorageKeys.filter((item: string) => item !== storageKey),
                                            )
                                        }
                                    >
                                        ×
                                    </Button>
                                </div>
                            ))}
                        </div>
                        <Upload
                            accept="image/*"
                            multiple
                            showUploadList={false}
                            beforeUpload={(file) => {
                                if (!file.type.startsWith("image/")) {
                                    message.error("只能上传图片文件");
                                    return Upload.LIST_IGNORE;
                                }
                                if (skuImageStorageKeys.length + skuUploadsInFlightRef.current >= 50) {
                                    message.error("单个 SKU 最多保存 50 张参考图");
                                    return Upload.LIST_IGNORE;
                                }
                                skuUploadFileSessions.current.set(file, skuUploadSession.current);
                                skuUploadsInFlightRef.current += 1;
                                setSKUUploadsInFlight(skuUploadsInFlightRef.current);
                                return true;
                            }}
                            customRequest={({ file, onSuccess, onError }) => {
                                const session = skuUploadFileSessions.current.get(file as object);
                                if (session !== skuUploadSession.current) return { abort: () => undefined };
                                const controller = new AbortController();
                                skuUploadControllers.current.add(controller);
                                void uploadWorkspaceFile(token, `image:${crypto.randomUUID()}`, file as File, controller.signal)
                                    .then((saved) => {
                                        if (session !== skuUploadSession.current) return;
                                        const current = skuForm.getFieldValue("imageStorageKeys") || [];
                                        skuForm.setFieldValue("imageStorageKeys", [...current, saved.storageKey]);
                                        onSuccess?.(saved);
                                    })
                                    .catch((error) => {
                                        if (session !== skuUploadSession.current || controller.signal.aborted) return;
                                        message.error(error instanceof Error ? error.message : "参考图上传失败");
                                        onError?.(error as Error);
                                    })
                                    .finally(() => {
                                        skuUploadControllers.current.delete(controller);
                                        if (session === skuUploadSession.current) {
                                            skuUploadsInFlightRef.current = Math.max(0, skuUploadsInFlightRef.current - 1);
                                            setSKUUploadsInFlight(skuUploadsInFlightRef.current);
                                        }
                                    });
                                return { abort: () => controller.abort() };
                            }}
                        >
                            <Button loading={skuUploadsInFlight > 0} disabled={skuImageStorageKeys.length + skuUploadsInFlight >= 50}>
                                上传参考图
                            </Button>
                        </Upload>
                    </Form.Item>
                    <Form.Item name="imageStorageKeys" hidden>
                        <Select mode="multiple" />
                    </Form.Item>
                </Form>
            </Modal>
            <Modal
                title="创建批量生产任务"
                open={batchOpen}
                width={720}
                confirmLoading={working}
                onCancel={() => {
                    setBatchOpen(false);
                    setPromptPreview("");
                }}
                onOk={async () => {
                    const value = await batchForm.validateFields();
                    const { previewSkuId: _previewSkuId, ...input } = value;
                    if (await run(() => createBatchProductionJob({ ...input, presetVersion: input.presetVersion || 1, requestId: batchRequestId.current }), "批量任务已进入队列")) setBatchOpen(false);
                }}
            >
                <Form form={batchForm} layout="vertical">
                    <Form.Item name="name" label="任务名称" rules={[{ required: true }]}>
                        <Input placeholder="例如：秋季上新主图" />
                    </Form.Item>
                    <Form.Item name="presetId" label="生产模板" rules={[{ required: true }]}>
                        <Select
                            loading={templatesQuery.isFetching}
                            options={productionTemplateOptions.map((item) => ({ value: item.id, label: `${item.title} · ${item.description}` }))}
                            onChange={(id) => batchForm.setFieldValue("presetVersion", productionTemplateOptions.find((item) => item.id === id)?.version || 1)}
                        />
                    </Form.Item>
                    <Form.Item name="presetVersion" hidden>
                        <Input />
                    </Form.Item>
                    <Form.Item name="deliverySpecId" label="渠道交付规格" rules={[{ required: true }]} extra="规格会随任务固化，并统一应用于结果像素、文件格式和压缩包命名。">
                        <Select
                            loading={deliverySpecsQuery.isFetching}
                            options={(deliverySpecsQuery.data || []).map((item) => ({
                                value: item.id,
                                label: item.id === "original" ? "通用 · 保留原始结果" : `${item.platform} · ${item.name} · ${item.width}×${item.height} · ${item.format.toUpperCase()}`,
                            }))}
                        />
                    </Form.Item>
                    <Form.Item name="brandId" label="品牌">
                        <Select allowClear showSearch filterOption={false} onSearch={setBrandOptionKeyword} loading={brandOptionsQuery.isFetching} options={brandOptions.map((item) => ({ value: item.id, label: item.name }))} />
                    </Form.Item>
                    <Form.Item name="productIds" label="商品" rules={[{ required: true }]}>
                        <Select
                            mode="multiple"
                            showSearch
                            filterOption={false}
                            onSearch={setProductOptionKeyword}
                            loading={productOptionsQuery.isFetching}
                            options={(productOptionsQuery.data?.items || []).map((item) => ({ value: item.id, label: `${item.name} · ${item.code}` }))}
                            onChange={() => batchForm.setFieldValue("previewSkuId", undefined)}
                        />
                    </Form.Item>
                    {batchProductIDs[0] ? (
                        <Form.Item name="previewSkuId" label="提示词预览 SKU" extra="可选；未选择时预览商品级提示词。批量执行仍会为每个 SKU 自动注入各自属性。">
                            <Select allowClear loading={previewSKUsQuery.isFetching} options={(previewSKUsQuery.data?.items || []).map((item) => ({ value: item.id, label: `${item.name} · ${item.code}` }))} />
                        </Form.Item>
                    ) : null}
                    <Button block icon={<ScanText className="size-4" />} onClick={() => void previewBatchPrompt()}>
                        预览最终提示词
                    </Button>
                </Form>
            </Modal>
            <Drawer title={`${selectedTemplate?.name || "模板"} · 版本历史`} width={720} open={Boolean(selectedTemplate)} onClose={() => setSelectedTemplate(null)}>
                <Table<ProductionTemplateVersion>
                    rowKey="id"
                    loading={templateVersionsQuery.isFetching}
                    dataSource={templateVersionsQuery.data || []}
                    pagination={false}
                    expandable={{ expandedRowRender: (item) => <pre className="whitespace-pre-wrap text-sm leading-6">{item.prompt}</pre> }}
                    columns={[
                        { title: "版本", width: 90, render: (_, item) => `v${item.version}` },
                        { title: "创建人", dataIndex: "createdBy" },
                        { title: "发布时间", dataIndex: "createdAt", width: 190 },
                    ]}
                />
            </Drawer>
            <Modal
                title="最终提示词预览"
                open={Boolean(promptPreview)}
                width={760}
                footer={
                    <Button type="primary" onClick={() => setPromptPreview("")}>
                        关闭
                    </Button>
                }
                onCancel={() => setPromptPreview("")}
            >
                <pre className="max-h-[60vh] overflow-y-auto whitespace-pre-wrap rounded-lg bg-muted p-4 text-sm leading-6">{promptPreview}</pre>
            </Modal>
            <Drawer
                title={`${selectedBatch?.name || "任务"} · 结果审核`}
                width={1180}
                open={Boolean(selectedBatch)}
                onClose={() => {
                    setSelectedBatch(null);
                    setReviewDraft(null);
                    setComparisonItems([]);
                    setComparisonOpen(false);
                }}
            >
                <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
                    <div className="flex flex-wrap gap-2">
                        <Input.Search
                            allowClear
                            placeholder="搜索商品、SKU、错误或批注"
                            className="w-72"
                            onSearch={(keyword) => {
                                setComparisonItems([]);
                                setItemListQuery((value) => ({ ...value, page: 1, keyword }));
                            }}
                        />
                        <Select
                            className="w-32"
                            value={itemStatus}
                            options={[
                                { value: "all", label: "全部状态" },
                                { value: "queued", label: "排队中" },
                                { value: "running", label: "生产中" },
                                { value: "completed", label: "已完成" },
                                { value: "failed", label: "失败" },
                                { value: "cancelled", label: "已取消" },
                            ]}
                            onChange={(value) => {
                                setComparisonItems([]);
                                setItemStatus(value);
                                setItemListQuery((current) => ({ ...current, page: 1 }));
                            }}
                        />
                    </div>
                    <Button icon={<Columns3 className="size-4" />} disabled={comparisonItems.length < 2} onClick={() => setComparisonOpen(true)}>
                        对比选片 {comparisonItems.length}/4
                    </Button>
                </div>
                <Table<BatchProductionItem>
                    rowKey="id"
                    loading={batchItemsQuery.isFetching}
                    dataSource={batchItemsQuery.data?.items || []}
                    rowSelection={{
                        hideSelectAll: true,
                        selectedRowKeys: comparisonItems.map((item) => item.id),
                        getCheckboxProps: (item) => ({ disabled: item.status !== "completed" || !item.resultStorageKey || (comparisonItems.length >= 4 && !comparisonItems.some((selected) => selected.id === item.id)) }),
                        onSelect: (item, selected) => setComparisonItems((current) => (selected ? [...current, item] : current.filter((currentItem) => currentItem.id !== item.id))),
                    }}
                    scroll={{ x: 1120 }}
                    pagination={{
                        current: itemListQuery.page,
                        pageSize: itemListQuery.pageSize,
                        total: batchItemsQuery.data?.total || 0,
                        showSizeChanger: true,
                        onChange: (page, pageSize) => {
                            setComparisonItems([]);
                            setItemListQuery((value) => ({ ...value, page, pageSize }));
                        },
                    }}
                    columns={[
                        {
                            title: "商品 / SKU",
                            width: 220,
                            render: (_, item) => (
                                <div>
                                    <div className="font-medium">{item.qualityContext?.product.name || item.productId}</div>
                                    <div className="mt-1 text-xs text-muted-foreground">{item.qualityContext?.sku ? `${item.qualityContext.sku.name} · ${item.qualityContext.sku.code}` : "按 SPU 生产"}</div>
                                </div>
                            ),
                        },
                        {
                            title: "结果",
                            width: 90,
                            render: (_, item) =>
                                item.resultStorageKey ? (
                                    <a href={workspaceFileUrl(item.resultStorageKey)} target="_blank" rel="noreferrer">
                                        <Avatar shape="square" size={56} src={workspaceFileUrl(item.resultStorageKey)} />
                                    </a>
                                ) : (
                                    "—"
                                ),
                        },
                        {
                            title: "生产",
                            width: 110,
                            render: (_, item) => (
                                <div className="space-y-1">
                                    <Tag color={statusColors[item.status]}>{statusLabels[item.status]}</Tag>
                                    <div className="text-xs text-muted-foreground">
                                        第 {item.runNumber} 轮 · {item.attempts} 次尝试
                                    </div>
                                </div>
                            ),
                        },
                        {
                            title: "审核",
                            width: 110,
                            render: (_, item) =>
                                item.reviewStatus ? (
                                    <div className="space-y-1">
                                        <Tag color={reviewColors[item.reviewStatus]}>{reviewLabels[item.reviewStatus]}</Tag>
                                        {item.isPrimary ? <Tag color="primary">商品主图</Tag> : null}
                                    </div>
                                ) : (
                                    "—"
                                ),
                        },
                        { title: "批注 / 错误", render: (_, item) => item.reviewComment || item.errorMessage || "—" },
                        {
                            title: "操作",
                            fixed: "right",
                            width: 360,
                            render: (_, item) => (
                                <div className="flex flex-wrap gap-1">
                                    {canReviewBatchItem(item) ? (
                                        <>
                                            <Button size="small" onClick={() => openReview(item, "approved")}>
                                                通过
                                            </Button>
                                            <Button danger size="small" onClick={() => openReview(item, "rejected")}>
                                                驳回
                                            </Button>
                                        </>
                                    ) : null}
                                    {canSetPrimary(item) ? (
                                        <Button size="small" onClick={() => void setPrimary(item)}>
                                            设为主图
                                        </Button>
                                    ) : null}
                                    {canWrite && item.reviewStatus === "approved" ? (
                                        <>
                                            <Button size="small" onClick={() => void saveBatchItemToAssets(item)}>
                                                存素材
                                            </Button>
                                            <Button size="small" onClick={() => void saveBatchItemToCanvas(item)}>
                                                建画布
                                            </Button>
                                        </>
                                    ) : null}
                                    {canWrite && (item.status === "failed" || item.reviewStatus === "rejected") ? (
                                        <Popconfirm title="重新生成该项？" description="旧结果将解除引用，并进入新的生产轮次。" onConfirm={() => void run(() => retryBatchProductionItem(item.jobId, item.id, item.runNumber), "生产项已重新排队")}>
                                            <Button size="small" icon={<RefreshCw className="size-3.5" />}>
                                                重新生成
                                            </Button>
                                        </Popconfirm>
                                    ) : null}
                                </div>
                            ),
                        },
                    ]}
                />
            </Drawer>
            {comparisonItems.length ? (
                <BatchResultComparison
                    open={comparisonOpen}
                    job={selectedBatch}
                    items={comparisonItems}
                    canReview={canReview}
                    working={working}
                    onClose={() => setComparisonOpen(false)}
                    onApprove={(item) => {
                        setComparisonOpen(false);
                        setComparisonItems([]);
                        openReview(item, "approved");
                    }}
                    onReject={(item) => {
                        setComparisonOpen(false);
                        setComparisonItems([]);
                        openReview(item, "rejected");
                    }}
                    onSetPrimary={async (item) => {
                        if (!canSetPrimary(item)) {
                            message.error("当前结果不可设为主图");
                            return;
                        }
                        const requestJobId = item.jobId;
                        if ((await setPrimary(item)) && selectedBatchIdRef.current === requestJobId)
                            setComparisonItems((current) => current.map((currentItem) => (currentItem.productId === item.productId && currentItem.skuId === item.skuId ? { ...currentItem, isPrimary: currentItem.id === item.id } : currentItem)));
                    }}
                />
            ) : null}
            <Modal
                title={reviewDraft?.status === "approved" ? "通过生产结果" : "驳回生产结果"}
                open={Boolean(reviewDraft)}
                confirmLoading={working}
                okText={reviewDraft?.status === "approved" ? "确认通过" : "确认驳回"}
                okButtonProps={{ danger: reviewDraft?.status === "rejected" }}
                onCancel={() => setReviewDraft(null)}
                onOk={submitReview}
            >
                <Form form={reviewForm} layout="vertical">
                    <Form.Item name="comment" label="审核批注" rules={[{ required: reviewDraft?.status === "rejected", message: "驳回时请填写批注" }, { max: 1000 }]}>
                        <Input.TextArea rows={4} placeholder={reviewDraft?.status === "rejected" ? "请说明需要调整的内容" : "可选，记录通过说明"} />
                    </Form.Item>
                </Form>
            </Modal>
        </div>
    );
}
