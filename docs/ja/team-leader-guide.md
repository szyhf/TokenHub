# チームリーダー LLM API 導入ガイド

Language: [English](../team-leader-guide.md) | [简体中文](../zh-CN/team-leader-guide.md) | 日本語

このガイドは、業務アプリケーションが Project 単位の TokenHub API Key で承認済み大規模言語モデルを呼び出せるようにするチームリーダー向けです。

## チームリーダーの責任

| 領域 | 管理するもの |
| --- | --- |
| Project | メンバー、Key、クォータ、コスト配賦の境界 |
| Members | Project 詳細パネルでアプリ責任者または開発者を追加 |
| API Keys | 利用量とコストを持つ Project で Key を発行 |
| Models | Key が意図したモデル一覧を見られるか検証 |
| Reports | メンバー、Project、モデル、Cost Center 別に利用量を確認 |

## チーム横断コラボレーション

1 つの Project に複数のチームを関連付けられます。Project の**プライマリチーム**は、既定の責任、コスト配賦、承認責任を担う単一主体のままです。Project owner も単一ユーザーのままです。別チームの関連付けはアクセス権だけを付与し、Project の API Key、モデル権限、クォータ、予算、ルーティングポリシーを複製しません。

各関連チームには 1 つの Project ロールがあります。

| チームの Project ロール | 有効な権限 |
| --- | --- |
| `viewer` | Project と閲覧可能なレポートを表示 |
| `developer` | viewer 権限に加え、現在のユーザーを owner とする Project Key を発行 |
| `maintainer` | developer 権限に加え、コンソールロールで許可された Project、メンバー、Key の管理 |

直接の Project メンバーロールと、関連するすべてのチームロールは `owner` > `maintainer` > `developer` > `viewer` の固定順で統合されます。複数の関連チームに所属するユーザーには最上位ロールが適用され、管理者は **User Management** でプライマリチームと追加チームを設定できます。既存の単一チーム Project は互換モードで移行され、同じチームの一般ユーザーに新しい権限を与えず、従来のチームリーダー権限を維持します。管理者は Project 詳細で互換ロールを置き換えられます。

**Project Spaces** で Project を選択し、**Linked Teams** からチームの追加、ロール変更、削除を行います。権限変更は次のリクエストから反映されます。ユーザーへの割り当てや Project への新規関連付けができるのは有効なチームだけです。関連チームを無効にすると、その Project ロールによるアクセス許可は直ちに停止します。別のプライマリチームを設定するまで現在のプライマリチームは削除できず、最後の関連チームも削除できません。Project またはユーザーから参照されているチーム自体も削除できません。

管理 API でも同じ操作ができます。

| メソッド | エンドポイント | 用途 |
| --- | --- | --- |
| `GET` | `/api/admin/projects/{project_id}/teams?limit=50&offset=0` | 関連チームをページングして一覧表示 |
| `POST` | `/api/admin/projects/{project_id}/teams` | `{ "team_id": "...", "role": "viewer|developer|maintainer" }` を関連付け |
| `PATCH` | `/api/admin/projects/{project_id}/teams/{team_id}` | 関連チームのロールを変更 |
| `DELETE` | `/api/admin/projects/{project_id}/teams/{team_id}` | プライマリでも最後でもないチーム関連を削除 |

## 自前のプロバイダーチャネル接続

チームリーダーは、管理者に代行してもらうことなく、自分の上流 LLM 設定をチーム所有のプロバイダーチャネルとして登録できます。

1. **プロバイダーチャネル**を開き、上流の Base URL と API キーでプロバイダーを作成します。チャネルには自動的に自分のチームが割り当てられ、チームリーダーがリクエストした `owner_team_id` は無視されます。
2. 作成時または作成後に上流モデルを取り込みます。取り込みだけでは呼び出し側に何も公開されません。
3. 複数のキー、重み、クールダウン分離が必要な場合は、チャネル配下にプロバイダーリソース（アカウント）を追加します。
4. すべてがチーム単位でスコープされます。自分のチャネルとリソースのみを閲覧・編集・テスト・削除でき、プラットフォームチャネルや他チームのチャネルは閲覧も管理もできません。

上流アクセスポリシーはプラットフォームチャネルと同様に適用されます。管理者が許可しない限りプライベートアドレスやループバックは拒否され、既存のプロバイダー ID の再利用も拒否されます。チャネルを削除するとルート、取り込んだモデル、リソース、観測データが一緒に削除されるため、再作成する場合は依存ルートを先に整理してください。

## Project Key の発行

1. **Project Spaces** で Project を作成または選択します。
2. Project をクリックし、右側メンバーパネルでアプリ責任者を追加します。
3. **Key Management** を開き、その Project で Key を作成します。
4. Key をアプリケーションに必要なモデルとクォータに制限します。
5. `GET /v1/models` で Key のモデル範囲を検証します。
6. 社内のシークレット運用に従って Key をアプリ責任者へ渡します。

## 利用可能モデルの検証

```bash
curl --request GET \
  --url "http://localhost:8080/v1/models" \
  --header "Authorization: Bearer PROJECT_API_KEY" \
  --header "Content-Type: application/json"
```

返された `data[].id` がアプリケーションで利用できるモデル ID です。

## Chat 呼び出しの検証

```bash
curl --request POST \
  --url "http://localhost:8080/v1/chat/completions" \
  --header "Authorization: Bearer PROJECT_API_KEY" \
  --header "Content-Type: application/json" \
  --data '{
    "model": "gpt-4.1-mini",
    "messages": [
      {"role": "user", "content": "Write a concise project onboarding checklist."}
    ],
    "stream": false
  }'
```

## ガバナンスチェック

| チェック | 重要な理由 |
| --- | --- |
| Project owner | 利用量とコストの責任者を明確にするため |
| Member role | 信頼できる Project メンバーだけが Key を発行またはローテーションするため |
| Model scope | Key が必要なモデルだけを公開するため |
| Quota | クォータと同時実行を想定トラフィックに合わせるため |
| Logs | 失敗リクエストを `request_id` で追跡するため |

## よくあるエラー

| ステータス | チームリーダーの対応 |
| --- | --- |
| 401 | アプリが有効な Project Key を使っているか確認 |
| 403 | Project メンバーと Key の許可モデル範囲を確認 |
| 429 | クォータ、同時実行、Key/Project 制限を確認 |
| 503 | 管理者にルートと Provider ヘルス確認を依頼 |
| 500 | Request Logs で `request_id` から上流エラーを確認 |

## スクリーンショット

![Gateway documentation](../assets/screenshots/gateway-en.png)
