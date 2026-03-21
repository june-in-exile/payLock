module orca::paywall {
    use sui::coin::{Self, Coin};
    use sui::sui::SUI;
    use sui::transfer;
    use sui::object::{Self, UID, ID};
    use sui::tx_context::{Self, TxContext};
    use std::string::String;

    // === Error codes ===
    const EInsufficientPayment: u64 = 0;
    const EVideoMismatch: u64 = 1;
    const EInvalidSealId: u64 = 2;
    const ENotCreator: u64 = 3;

    // === Structs ===

    /// Video metadata, created by the content creator.
    /// Shared object so anyone can reference it for purchase and seal_approve.
    public struct Video has key {
        id: UID,
        price: u64,
        creator: address,
        preview_blob_id: String,
        full_blob_id: String,
    }

    /// Proof of purchase, minted after payment. Owned by the buyer.
    public struct AccessPass has key, store {
        id: UID,
        video_id: ID,
    }

    // === Public functions ===

    /// Creator publishes a new video with preview and full blob IDs.
    public fun create_video(
        price: u64,
        preview_blob_id: String,
        full_blob_id: String,
        ctx: &mut TxContext,
    ) {
        let video = Video {
            id: object::new(ctx),
            price,
            creator: tx_context::sender(ctx),
            preview_blob_id,
            full_blob_id,
        };
        transfer::share_object(video);
    }

    /// User pays to unlock a video. Mints an AccessPass on success.
    public fun purchase(
        video: &Video,
        payment: Coin<SUI>,
        ctx: &mut TxContext,
    ): AccessPass {
        assert!(coin::value(&payment) >= video.price, EInsufficientPayment);

        // Transfer full payment to creator
        transfer::public_transfer(payment, video.creator);

        let pass = AccessPass {
            id: object::new(ctx),
            video_id: object::id(video),
        };
        pass
    }

    /// Creator updates the full blob ID after Seal encryption + Walrus upload.
    public fun update_full_blob_id(
        video: &mut Video,
        full_blob_id: String,
        ctx: &TxContext,
    ) {
        assert!(tx_context::sender(ctx) == video.creator, ENotCreator);
        video.full_blob_id = full_blob_id;
    }

    /// Convenience entry function: purchases and transfers AccessPass to buyer.
    entry fun purchase_and_transfer(
        video: &Video,
        payment: Coin<SUI>,
        ctx: &mut TxContext,
    ) {
        let pass = purchase(video, payment, ctx);
        transfer::public_transfer(pass, tx_context::sender(ctx));
    }

    /// Seal key server calls this to verify decryption rights.
    /// Validates that:
    /// 1. The AccessPass belongs to the referenced Video
    /// 2. The seal `id` starts with the Video object's ID bytes (namespace prefix)
    entry fun seal_approve(
        id: vector<u8>,
        pass: &AccessPass,
        video: &Video,
    ) {
        assert!(pass.video_id == object::id(video), EVideoMismatch);

        let video_id_bytes = object::id_bytes(video);
        let prefix_len = vector::length(&video_id_bytes);
        let id_len = vector::length(&id);
        assert!(id_len >= prefix_len, EInvalidSealId);

        let mut i = 0;
        while (i < prefix_len) {
            assert!(
                *vector::borrow(&id, i) == *vector::borrow(&video_id_bytes, i),
                EInvalidSealId,
            );
            i = i + 1;
        };
    }

    // === Accessors (for testing and frontend queries) ===

    public fun video_price(video: &Video): u64 { video.price }
    public fun video_creator(video: &Video): address { video.creator }
    public fun video_preview_blob_id(video: &Video): &String { &video.preview_blob_id }
    public fun video_full_blob_id(video: &Video): &String { &video.full_blob_id }
    public fun access_pass_video_id(pass: &AccessPass): ID { pass.video_id }
}
