import { tx } from "../i18n/runtime";
export function ProviderEmbeddingFields({ values, onUpdate }: { values: Record<string, string>; onUpdate: (key: string, value: string) => void }) {
  return <details className="provider-account-runtime">
    <summary><strong>{tx("文本 Embedding 配置")}</strong></summary>
    <div className="provider-form-grid">
      <label className="field"><span>{tx("Embedding 协议")}</span>
        <select value={values.embedding_protocol ?? ""} onChange={(event) => onUpdate("embedding_protocol", event.target.value)}>
          <option value="">{tx("自动选择")}</option>
          {["openai", "cohere", "jina", "voyage", "dashscope", "tei"].map((protocol) => <option key={protocol} value={protocol}>{protocol}</option>)}
        </select>
      </label>
      <label className="field"><span>{tx("Embedding 接口路径")}</span><input value={values.embedding_path ?? ""} placeholder="/embeddings" onChange={(event) => onUpdate("embedding_path", event.target.value)} /></label>
      <label className="field"><span>{tx("向量空间映射")}</span><textarea value={values.embedding_spaces ?? ""} placeholder={'{"upstream-model":"verified-space-id"}'} onChange={(event) => onUpdate("embedding_spaces", event.target.value)} /></label>
    </div>
    <p className="field-help">{tx("路径相对于 Base URL。只有确认兼容的模型才能填写相同向量空间标识；维度相同不代表兼容。更换空间需要重建索引。")}</p>
  </details>;
}
