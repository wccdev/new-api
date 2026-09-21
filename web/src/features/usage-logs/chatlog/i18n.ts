/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import i18next, { type i18n as I18nInstance } from 'i18next'

// The chatlog feature keeps its translations here instead of the shared locale
// files so that merging upstream locale changes never conflicts with it.
// Keys are the English source strings; language codes match `@/i18n/config`.
const CHATLOG_NAMESPACE = 'translation'

type ChatlogTranslations = Record<string, string>

export const chatlogResources: Record<string, ChatlogTranslations> = {
  en: {
    'View conversation': 'View conversation',
    Conversation: 'Conversation',
    'View the full conversation recorded for this request':
      'View the full conversation recorded for this request',
    'No conversation record for this request':
      'No conversation record for this request',
    'Conversation recording may be disabled, or the record has expired.':
      'Conversation recording may be disabled, or the record has expired.',
    'Failed to load the conversation record':
      'Failed to load the conversation record',
    'No parsed messages. Check the Raw JSON tab.':
      'No parsed messages. Check the Raw JSON tab.',
    Assistant: 'Assistant',
    Tool: 'Tool',
    'Tool call': 'Tool call',
    'Tool call ID': 'Tool call ID',
    Truncated: 'Truncated',
    'This record exceeded the size limit and was truncated':
      'This record exceeded the size limit and was truncated',
    Protocol: 'Protocol',
    'Copy message': 'Copy message',
    'Copy request JSON': 'Copy request JSON',
    'Copy response JSON': 'Copy response JSON',
    'View image {{index}}': 'View image {{index}}',
    'Conversation image {{index}}': 'Conversation image {{index}}',
  },
  zhCN: {
    'View conversation': '查看对话',
    Conversation: '对话',
    'View the full conversation recorded for this request':
      '查看该请求记录的完整对话',
    'No conversation record for this request': '该请求没有对话记录',
    'Conversation recording may be disabled, or the record has expired.':
      '可能未开启对话记录，或记录已过期。',
    'Failed to load the conversation record': '加载对话记录失败',
    'No parsed messages. Check the Raw JSON tab.':
      '没有可解析的消息，请查看“原始 JSON”标签页。',
    Assistant: '助手',
    Tool: '工具',
    'Tool call': '工具调用',
    'Tool call ID': '工具调用 ID',
    Truncated: '已截断',
    'This record exceeded the size limit and was truncated':
      '该记录超出大小上限，内容已被截断',
    Protocol: '协议',
    'Copy message': '复制消息',
    'Copy request JSON': '复制请求 JSON',
    'Copy response JSON': '复制响应 JSON',
    'View image {{index}}': '查看图片 {{index}}',
    'Conversation image {{index}}': '对话图片 {{index}}',
  },
  zhTW: {
    'View conversation': '檢視對話',
    Conversation: '對話',
    'View the full conversation recorded for this request':
      '檢視該請求記錄的完整對話',
    'No conversation record for this request': '該請求沒有對話記錄',
    'Conversation recording may be disabled, or the record has expired.':
      '可能未開啟對話記錄，或記錄已過期。',
    'Failed to load the conversation record': '載入對話記錄失敗',
    'No parsed messages. Check the Raw JSON tab.':
      '沒有可解析的訊息，請查看「原始 JSON」分頁。',
    Assistant: '助理',
    Tool: '工具',
    'Tool call': '工具呼叫',
    'Tool call ID': '工具呼叫 ID',
    Truncated: '已截斷',
    'This record exceeded the size limit and was truncated':
      '該記錄超出大小上限，內容已被截斷',
    Protocol: '協定',
    'Copy message': '複製訊息',
    'Copy request JSON': '複製請求 JSON',
    'Copy response JSON': '複製回應 JSON',
    'View image {{index}}': '檢視圖片 {{index}}',
    'Conversation image {{index}}': '對話圖片 {{index}}',
  },
  fr: {
    'View conversation': 'Voir la conversation',
    Conversation: 'Conversation',
    'View the full conversation recorded for this request':
      'Voir la conversation complète enregistrée pour cette requête',
    'No conversation record for this request':
      'Aucune conversation enregistrée pour cette requête',
    'Conversation recording may be disabled, or the record has expired.':
      "L'enregistrement des conversations est peut-être désactivé, ou l'enregistrement a expiré.",
    'Failed to load the conversation record':
      "Échec du chargement de l'enregistrement de la conversation",
    'No parsed messages. Check the Raw JSON tab.':
      "Aucun message analysé. Consultez l'onglet JSON brut.",
    Assistant: 'Assistant',
    Tool: 'Outil',
    'Tool call': "Appel d'outil",
    'Tool call ID': "ID de l'appel d'outil",
    Truncated: 'Tronqué',
    'This record exceeded the size limit and was truncated':
      'Cet enregistrement dépassait la taille limite et a été tronqué',
    Protocol: 'Protocole',
    'Copy message': 'Copier le message',
    'Copy request JSON': 'Copier le JSON de la requête',
    'Copy response JSON': 'Copier le JSON de la réponse',
    'View image {{index}}': "Voir l'image {{index}}",
    'Conversation image {{index}}': 'Image de la conversation {{index}}',
  },
  ru: {
    'View conversation': 'Просмотреть диалог',
    Conversation: 'Диалог',
    'View the full conversation recorded for this request':
      'Просмотр полного диалога, записанного для этого запроса',
    'No conversation record for this request':
      'Для этого запроса нет записи диалога',
    'Conversation recording may be disabled, or the record has expired.':
      'Запись диалогов может быть отключена, либо срок хранения записи истёк.',
    'Failed to load the conversation record':
      'Не удалось загрузить запись диалога',
    'No parsed messages. Check the Raw JSON tab.':
      'Нет разобранных сообщений. Откройте вкладку «Исходный JSON».',
    Assistant: 'Ассистент',
    Tool: 'Инструмент',
    'Tool call': 'Вызов инструмента',
    'Tool call ID': 'ID вызова инструмента',
    Truncated: 'Обрезано',
    'This record exceeded the size limit and was truncated':
      'Запись превысила лимит размера и была обрезана',
    Protocol: 'Протокол',
    'Copy message': 'Копировать сообщение',
    'Copy request JSON': 'Копировать JSON запроса',
    'Copy response JSON': 'Копировать JSON ответа',
    'View image {{index}}': 'Открыть изображение {{index}}',
    'Conversation image {{index}}': 'Изображение диалога {{index}}',
  },
  ja: {
    'View conversation': '会話を表示',
    Conversation: '会話',
    'View the full conversation recorded for this request':
      'このリクエストで記録された会話の全文を表示',
    'No conversation record for this request':
      'このリクエストの会話記録はありません',
    'Conversation recording may be disabled, or the record has expired.':
      '会話の記録が無効になっているか、記録の保存期限が切れています。',
    'Failed to load the conversation record':
      '会話記録の読み込みに失敗しました',
    'No parsed messages. Check the Raw JSON tab.':
      '解析できたメッセージがありません。「生の JSON」タブを確認してください。',
    Assistant: 'アシスタント',
    Tool: 'ツール',
    'Tool call': 'ツール呼び出し',
    'Tool call ID': 'ツール呼び出し ID',
    Truncated: '切り詰め済み',
    'This record exceeded the size limit and was truncated':
      'この記録はサイズ上限を超えたため切り詰められました',
    Protocol: 'プロトコル',
    'Copy message': 'メッセージをコピー',
    'Copy request JSON': 'リクエスト JSON をコピー',
    'Copy response JSON': 'レスポンス JSON をコピー',
    'View image {{index}}': '画像 {{index}} を表示',
    'Conversation image {{index}}': '会話の画像 {{index}}',
  },
  vi: {
    'View conversation': 'Xem hội thoại',
    Conversation: 'Hội thoại',
    'View the full conversation recorded for this request':
      'Xem toàn bộ hội thoại đã ghi lại cho yêu cầu này',
    'No conversation record for this request':
      'Không có bản ghi hội thoại cho yêu cầu này',
    'Conversation recording may be disabled, or the record has expired.':
      'Tính năng ghi hội thoại có thể đang tắt, hoặc bản ghi đã hết hạn.',
    'Failed to load the conversation record':
      'Không tải được bản ghi hội thoại',
    'No parsed messages. Check the Raw JSON tab.':
      'Không có tin nhắn nào được phân tích. Hãy xem tab JSON thô.',
    Assistant: 'Trợ lý',
    Tool: 'Công cụ',
    'Tool call': 'Lệnh gọi công cụ',
    'Tool call ID': 'ID lệnh gọi công cụ',
    Truncated: 'Đã cắt bớt',
    'This record exceeded the size limit and was truncated':
      'Bản ghi này vượt quá giới hạn kích thước và đã bị cắt bớt',
    Protocol: 'Giao thức',
    'Copy message': 'Sao chép tin nhắn',
    'Copy request JSON': 'Sao chép JSON yêu cầu',
    'Copy response JSON': 'Sao chép JSON phản hồi',
    'View image {{index}}': 'Xem hình ảnh {{index}}',
    'Conversation image {{index}}': 'Hình ảnh hội thoại {{index}}',
  },
}

/** Existing keys win, so shared locale files can override these later. */
export function registerChatlogTranslations(instance: I18nInstance): void {
  for (const [language, translations] of Object.entries(chatlogResources)) {
    instance.addResourceBundle(
      language,
      CHATLOG_NAMESPACE,
      translations,
      true,
      false
    )
  }
}

// The resource store only exists after init, and this module may be evaluated
// before `@/i18n/config` has run.
if (i18next.isInitialized) {
  registerChatlogTranslations(i18next)
} else {
  i18next.on('initialized', () => registerChatlogTranslations(i18next))
}
