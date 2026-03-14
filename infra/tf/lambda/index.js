exports.handler = async (event) => {
  for (const record of event.Records || []) {
    console.log("received ingest event", record.body);
  }

  return {
    batchItemFailures: [],
  };
};
