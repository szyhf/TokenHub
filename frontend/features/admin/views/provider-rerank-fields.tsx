import { tx } from "../i18n/runtime";
export function ProviderRerankFields({ values, onUpdate }: { values: Record<string, string>; onUpdate: (key: string, value: string) => void }) {
  return <details className="provider-account-runtime">
    <summary><strong>{tx("文本重排配置")}</strong></summary>
    <div className="provider-form-grid">
      <label className="field"><span>{tx("重排协议")}</span><select value={values.rerank_protocol ?? ""} onChange={(event) => onUpdate("rerank_protocol", event.target.value)}>
        <option value="">{tx("自动选择")}</option>
        {["jina", "cohere", "voyage", "qwen", "dashscope", "tei"].map((protocol) => <option key={protocol} value={protocol}>{protocol}</option>)}
      </select></label>
      <label className="field"><span>{tx("重排接口路径")}</span><input value={values.rerank_path ?? ""} placeholder="/rerank" onChange={(event) => onUpdate("rerank_path", event.target.value)} /></label>
    </div>
    <p>{tx("本地 BGE、vLLM 或 Xinference 可选择兼容的 jina 协议。阿里云按模型选择 qwen 或 dashscope，并核实地域地址。")}</p>
  </details>;
}
