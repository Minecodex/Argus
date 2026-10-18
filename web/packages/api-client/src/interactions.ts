import type { components } from "./generated/interactionapi";
export type ConversationAccessMode = "request_approval" | "full_access";
export type ConversationInteraction =
  components["schemas"]["ConversationInteraction"];
export type ConversationInteractionState =
  components["schemas"]["ConversationInteractionPage"];
export type InteractionResponse = components["schemas"]["InteractionResponse"];
export type InteractionAnswer = components["schemas"]["InteractionAnswer"];
export type InteractionQuestion = components["schemas"]["InteractionQuestion"];
export type GuidancePlan = components["schemas"]["GuidancePlan"];
export interface InteractionDomains {
  interactions: {
    list(conversationId: string): Promise<ConversationInteractionState>;
    get(conversationId: string, id: string): Promise<ConversationInteraction>;
    respond(
      conversationId: string,
      id: string,
      response: InteractionResponse,
    ): Promise<ConversationInteraction>;
  };
}
