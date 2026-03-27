module paylock::gating {
    use sui::coin::{Self, Coin};
    use sui::event;
    use sui::sui::SUI;
    use std::string::String;


    // === Error codes ===
    const EInsufficientPayment: u64 = 0;
    const EVideoMismatch: u64 = 1;
    const EInvalidSealId: u64 = 2;
    const EMissingSealNamespace: u64 = 3;
    const ENotCreator: u64 = 4;

    // === Structs ===

    /// Video metadata, created by the content creator.
    /// Shared object so anyone can reference it for purchase and seal_approve.
    public struct Video has key {
        id: UID,
        title: String,
        price: u64,
        creator: address,
        thumbnail_blob_id: String,
        preview_blob_id: String,
        full_blob_id: String,
        seal_namespace: vector<u8>,
    }

    /// Emitted when a new video is created, so off-chain watchers can detect it.
    public struct VideoCreated has copy, drop {
        video_id: ID,
        title: String,
        price: u64,
        creator: address,
        preview_blob_id: String,
        full_blob_id: String,
    }

    /// Emitted when a video is deleted by its creator.
    public struct VideoDeleted has copy, drop {
        video_id: ID,
        creator: address,
    }

    /// Proof of purchase, minted after payment. Owned by the buyer.
    public struct AccessPass has key, store {
        id: UID,
        video_id: ID,
    }

    // === Public functions ===

    /// Creator publishes a new video with preview and full blob IDs.
    /// For paid videos (price > 0), seal_namespace must be non-empty.
    public fun create_video(
        title: String,
        price: u64,
        thumbnail_blob_id: String,
        preview_blob_id: String,
        full_blob_id: String,
        seal_namespace: vector<u8>,
        ctx: &mut TxContext,
    ) {
        if (price > 0) {
            assert!(vector::length(&seal_namespace) > 0, EMissingSealNamespace);
        };

        let video = Video {
            id: object::new(ctx),
            title,
            price,
            creator: tx_context::sender(ctx),
            thumbnail_blob_id,
            preview_blob_id,
            full_blob_id,
            seal_namespace,
        };

        event::emit(VideoCreated {
            video_id: object::id(&video),
            title,
            price,
            creator: tx_context::sender(ctx),
            preview_blob_id,
            full_blob_id,
        });

        transfer::share_object(video);
    }

    /// Creator deletes their video. Destroys the shared Video object.
    /// Only the original creator can call this.
    entry fun delete_video(
        video: Video,
        ctx: &TxContext,
    ) {
        assert!(tx_context::sender(ctx) == video.creator, ENotCreator);

        let creator = video.creator;
        let video_id = object::id(&video);

        event::emit(VideoDeleted {
            video_id,
            creator,
        });

        let Video { id, title: _, price: _, creator: _, thumbnail_blob_id: _, preview_blob_id: _, full_blob_id: _, seal_namespace: _ } = video;
        object::delete(id);
    }

    /// User pays to unlock a video. Mints an AccessPass on success.
    public fun purchase(
        video: &Video,
        payment: &mut Coin<SUI>,
        ctx: &mut TxContext,
    ): AccessPass {
        assert!(coin::value(payment) >= video.price, EInsufficientPayment);

        // Transfer full payment to creator
        let paid = coin::split(payment, video.price, ctx);
        transfer::public_transfer(paid, video.creator);

        let pass = AccessPass {
            id: object::new(ctx),
            video_id: object::id(video),
        };
        pass
    }

    /// Convenience entry function: purchases and transfers AccessPass to buyer.
    /// Takes payment coin by value so the wallet correctly shows the outflow.
    entry fun purchase_and_transfer(
        video: &Video,
        mut payment: Coin<SUI>,
        ctx: &mut TxContext,
    ) {
        let pass = purchase(video, &mut payment, ctx);
        transfer::public_transfer(pass, tx_context::sender(ctx));

        // Return any remaining balance to the buyer
        if (coin::value(&payment) > 0) {
            transfer::public_transfer(payment, tx_context::sender(ctx));
        } else {
            coin::destroy_zero(payment);
        };
    }

    /// Seal key server calls this to verify decryption rights.
    /// Validates that:
    /// 1. The AccessPass belongs to the referenced Video
    /// 2. The seal `id` starts with the Video's seal_namespace bytes (prefix match)
    entry fun seal_approve(
        id: vector<u8>,
        pass: &AccessPass,
        video: &Video,
    ) {
        assert!(pass.video_id == object::id(video), EVideoMismatch);

        let prefix = &video.seal_namespace;
        let prefix_len = vector::length(prefix);
        let id_len = vector::length(&id);
        assert!(id_len >= prefix_len, EInvalidSealId);

        let mut i = 0;
        while (i < prefix_len) {
            assert!(
                *vector::borrow(&id, i) == *vector::borrow(prefix, i),
                EInvalidSealId,
            );
            i = i + 1;
        };
    }

    /// Seal key server calls this to verify decryption rights for the video creator.
    /// The creator can decrypt without an AccessPass.
    entry fun seal_approve_owner(
        id: vector<u8>,
        video: &Video,
        ctx: &TxContext,
    ) {
        assert!(tx_context::sender(ctx) == video.creator, ENotCreator);

        let prefix = &video.seal_namespace;
        let prefix_len = vector::length(prefix);
        let id_len = vector::length(&id);
        assert!(id_len >= prefix_len, EInvalidSealId);

        let mut i = 0;
        while (i < prefix_len) {
            assert!(
                *vector::borrow(&id, i) == *vector::borrow(prefix, i),
                EInvalidSealId,
            );
            i = i + 1;
        };
    }

    // === Accessors (for testing and frontend queries) ===

    public fun video_title(video: &Video): &String { &video.title }
    public fun video_price(video: &Video): u64 { video.price }
    public fun video_creator(video: &Video): address { video.creator }
    public fun video_thumbnail_blob_id(video: &Video): &String { &video.thumbnail_blob_id }
    public fun video_preview_blob_id(video: &Video): &String { &video.preview_blob_id }
    public fun video_full_blob_id(video: &Video): &String { &video.full_blob_id }
    public fun video_seal_namespace(video: &Video): &vector<u8> { &video.seal_namespace }
    public fun access_pass_video_id(pass: &AccessPass): ID { pass.video_id }
}
