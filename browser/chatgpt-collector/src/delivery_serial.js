export class DeliverySerializer {
  constructor() {
    this.chain = Promise.resolve();
  }

  run(operation) {
    const next = this.chain.then(operation, operation);
    this.chain = next.catch(() => {});
    return next;
  }
}
