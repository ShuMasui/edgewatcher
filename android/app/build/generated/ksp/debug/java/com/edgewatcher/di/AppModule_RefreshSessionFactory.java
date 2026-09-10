package com.edgewatcher.di;

import com.edgewatcher.domain.port.CredentialStore;
import com.edgewatcher.domain.port.ObservationApi;
import com.edgewatcher.domain.port.ObservationBuffer;
import com.edgewatcher.usecase.RefreshSessionUseCase;
import dagger.internal.DaggerGenerated;
import dagger.internal.Factory;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import dagger.internal.QualifierMetadata;
import dagger.internal.ScopeMetadata;
import javax.annotation.processing.Generated;

@ScopeMetadata
@QualifierMetadata
@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class AppModule_RefreshSessionFactory implements Factory<RefreshSessionUseCase> {
  private final Provider<ObservationApi> apiProvider;

  private final Provider<CredentialStore> storeProvider;

  private final Provider<ObservationBuffer> bufferProvider;

  private AppModule_RefreshSessionFactory(Provider<ObservationApi> apiProvider,
      Provider<CredentialStore> storeProvider, Provider<ObservationBuffer> bufferProvider) {
    this.apiProvider = apiProvider;
    this.storeProvider = storeProvider;
    this.bufferProvider = bufferProvider;
  }

  @Override
  public RefreshSessionUseCase get() {
    return refreshSession(apiProvider.get(), storeProvider.get(), bufferProvider.get());
  }

  public static AppModule_RefreshSessionFactory create(Provider<ObservationApi> apiProvider,
      Provider<CredentialStore> storeProvider, Provider<ObservationBuffer> bufferProvider) {
    return new AppModule_RefreshSessionFactory(apiProvider, storeProvider, bufferProvider);
  }

  public static RefreshSessionUseCase refreshSession(ObservationApi api, CredentialStore store,
      ObservationBuffer buffer) {
    return Preconditions.checkNotNullFromProvides(AppModule.INSTANCE.refreshSession(api, store, buffer));
  }
}
