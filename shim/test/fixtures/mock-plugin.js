module.exports = {
	default: {
		register(api) {
			api.registerTool({
				name: "mock_tool",
				description: "A mock tool for testing",
				parameters: { type: "object", properties: { input: { type: "string" } } },
				execute: async (callId, args) => {
					return { text: `echo: ${args.input || "none"}` };
				},
			});
		},
	},
};
